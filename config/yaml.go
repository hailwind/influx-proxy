package config

import (
	"fmt"
	"io/ioutil"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const (
	VERSION = "1.1"
)

type Node struct {
	Name         string
	ListenAddr   string
	Zone         string
	DB           string
	Nexts        string
	Interval     int
	IdleTimeout  int
	WriteTracing int
	QueryTracing int
}

type Backend struct {
	Name            string
	Url             string
	Db              string
	Zone            string
	Interval        int
	Timeout         int
	TimeoutQuery    int
	MaxRowLimit     int
	CheckInterval   int
	RewriteInterval int
	WriteOnly       int
}

type Measurement struct {
	Name     string
	Backends []string
}

type JudgeConfig struct {
	Enabled     bool
	Batch       int
	ConnTimeout int
	CallTimeout int
	MaxConns    int
	MaxIdle     int
	Replicas    int
	EndpointTag string
	Cluster     map[string]string
	ClusterList map[string]*ClusterNode
	MetricMap   map[string]string
	TagMap      map[string]string
	DropTag     []string
}

type ClusterNode struct {
	Addrs []string `json:"addrs"`
}

type Conf struct {
	Nodes        []Node
	Measurements []Measurement
	Backends     []Backend
	Judge        *JudgeConfig
}

func (c *Conf) GetConf(configPath string) *Conf {
	if configPath == "" {
		configPath = "config.yaml"
	}
	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Println(err.Error())
	}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		fmt.Println(err.Error())
	}

	c.Judge.ClusterList = formatClusterItems(c.Judge.Cluster)
	return c
}

func NewClusterNode(addrs []string) *ClusterNode {
	return &ClusterNode{addrs}
}

// map["node"]="host1,host2" --> map["node"]=["host1", "host2"]
func formatClusterItems(cluster map[string]string) map[string]*ClusterNode {
	ret := make(map[string]*ClusterNode)
	for node, clusterStr := range cluster {
		items := strings.Split(clusterStr, ",")
		nitems := make([]string, 0)
		for _, item := range items {
			nitems = append(nitems, strings.TrimSpace(item))
		}
		ret[node] = NewClusterNode(nitems)
	}

	return ret
}

// func main() {
// 	var c Conf
// 	conf := c.GetConf()
// 	// for _, backend := range conf.Backends {
// 	// 	fmt.Println(backend.Name)
// 	// }
// 	for _, measurement := range conf.Measurements {
// 		fmt.Println(measurement.Name)
// 		fmt.Println(measurement.Backends)
// 	}
// }
