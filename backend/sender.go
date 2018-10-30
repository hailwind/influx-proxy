package backend

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hailwind/influx-proxy/config"
	"github.com/toolkits/concurrent/semaphore"
	nlist "github.com/toolkits/container/list"
	nset "github.com/toolkits/container/set"
)

const (
	DefaultSendQueueMaxSize      = 102400                //10.24w
	DefaultSendTaskSleepInterval = time.Millisecond * 50 //默认睡眠间隔为50ms
)

var (
	MinStep int //最小上报周期,单位sec

	JudgeConfig    *config.JudgeConfig
	JudgeQueues    = make(map[string]*nlist.SafeListLimited)
	JudgeNodeRing  *ConsistentHashNodeRing
	JudgeConnPools *SafeRpcConnPools
)

type JudgeItem struct {
	Endpoint  string            `json:"endpoint"`
	Metric    string            `json:"metric"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	JudgeType string            `json:"judgeType"`
	Tags      map[string]string `json:"tags"`
}

// code == 0 => success
// code == 1 => bad request
type SimpleRpcResponse struct {
	Code int `json:"code"`
}

func (t *JudgeItem) PK() string {
	return PK(t.Endpoint, t.Metric, t.Tags)
}

func StartJudgeTask(judge *config.JudgeConfig) {
	JudgeConfig = judge
	MinStep = 30 //默认30s

	initConnPools()
	initSendQueues()
	initNodeRings()

	// SendTask
	startSendTask()

	log.Println("send.Start, ok")
}

func initConnPools() {
	judge := JudgeConfig
	judgeInstances := nset.NewStringSet()
	for _, instance := range judge.Cluster {
		judgeInstances.Add(instance)
	}

	JudgeConnPools = CreateSafeRpcConnPools(judge.MaxConns, judge.MaxIdle,
		judge.ConnTimeout, judge.CallTimeout, judgeInstances.ToSlice())

}

func initSendQueues() {
	judge := JudgeConfig

	for node, _ := range judge.Cluster {
		Q := nlist.NewSafeListLimited(DefaultSendQueueMaxSize)
		JudgeQueues[node] = Q
	}
}

func initNodeRings() {
	judge := JudgeConfig

	JudgeNodeRing = newConsistentHashNodesRing(judge.Replicas, KeysOfMap(judge.Cluster))
}

func startSendTask() {
	judge := JudgeConfig
	// init semaphore
	judgeConcurrent := JudgeConfig.MaxConns

	if judgeConcurrent < 1 {
		judgeConcurrent = 1
	}

	// init send go-routines
	for node, _ := range judge.Cluster {
		queue := JudgeQueues[node]
		go forward2JudgeTask(queue, node, judgeConcurrent)
	}

}

// 将数据 打入Judge的发送缓存队列
func Push2JudgeSendQueue(p []byte) {
	if JudgeConfig.Enabled {
		items := convert2JudgeItem(p)
		for _, judgeItem := range items {

			pk := judgeItem.PK()
			node, err := JudgeNodeRing.GetNode(pk)
			if err != nil {
				log.Println("E:", err)
				continue
			}
			Q := JudgeQueues[node]
			isSuccess := Q.PushFront(judgeItem)
			if !isSuccess {
				log.Println("Push to Queue fail.")
			}
		}
	}
}

// 转化为judge格式
func convert2JudgeItem(p []byte) []*JudgeItem {
	var items []*JudgeItem

	lines := strings.Split(string(p[:]), "\n")
	for _, line := range lines {
		// log.Println(line)
		data := strings.Split(line, " ")
		length := len(data)
		if length != 3 {
			continue
		}
		endpoint, tags := parseTags(data[0])
		ts, _ := strconv.ParseInt(data[length-1], 10, 64)

		metrics := strings.Split(data[1], ",")
		for _, item := range metrics {
			kv := strings.Split(item, "=")
			metric := kv[0]

			newKey, ok := JudgeConfig.MetricMap[metric]
			if ok {
				metric = newKey
			}

			v_len := len(kv[1])
			value, _ := strconv.ParseFloat(kv[1][:v_len-1], 64)
			judgeItem := &JudgeItem{
				Endpoint:  endpoint,
				Metric:    metric,
				Value:     value,
				Timestamp: ts / (1000 * 1000 * 1000),
				JudgeType: "GAUGE",
				Tags:      tags,
			}
			// data, _ := json.Marshal(judgeItem)
			// log.Println(string(data))
			items = append(items, judgeItem)
		}
	}

	return items
}

func parseTags(tagStr string) (endpoint string, tags map[string]string) {
	tagArr := strings.Split(tagStr, ",")

	tags = make(map[string]string)
	for _, item := range tagArr {
		kv := strings.Split(item, "=")
		if len(kv) == 2 {
			key := kv[0]
			if key == JudgeConfig.EndpointTag {
				endpoint = kv[1]
			}
			if b, _ := Contain(key, JudgeConfig.DropTag); !b {
				newKey, ok := JudgeConfig.TagMap[key]
				if ok {
					tags[newKey] = kv[1]
				} else {
					tags[key] = kv[1]
				}
			}
		}
	}
	return
}

func alignTs(ts int64, period int64) int64 {
	return ts - ts%period
}

func DestroyConnPools() {
	JudgeConnPools.Destroy()
}

// Judge定时任务, 将 Judge发送缓存中的数据 通过rpc连接池 发送到Judge
func forward2JudgeTask(Q *nlist.SafeListLimited, node string, concurrent int) {
	batch := JudgeConfig.Batch // 一次发送,最多batch条数据
	addr := JudgeConfig.Cluster[node]
	sema := semaphore.NewSemaphore(concurrent)

	for {
		items := Q.PopBackBy(batch)
		count := len(items)
		if count == 0 {
			time.Sleep(DefaultSendTaskSleepInterval)
			continue
		}

		judgeItems := make([]*JudgeItem, count)
		for i := 0; i < count; i++ {
			judgeItems[i] = items[i].(*JudgeItem)
		}

		//	同步Call + 有限并发 进行发送
		sema.Acquire()
		go func(addr string, judgeItems []*JudgeItem, count int) {
			defer sema.Release()

			resp := &SimpleRpcResponse{}
			var err error
			sendOk := false
			for i := 0; i < 3; i++ { //最多重试3次
				err = JudgeConnPools.Call(addr, "Judge.Send", judgeItems, resp)
				if err == nil {
					sendOk = true
					break
				}
				time.Sleep(time.Millisecond * 10)
			}
			// statistics
			if !sendOk {
				log.Printf("send judge %s:%s fail: %v ", node, addr, err)
			}
		}(addr, judgeItems, count)
	}
}
