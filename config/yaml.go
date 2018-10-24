package config

import (
	"fmt"
	"io/ioutil"

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

type Conf struct {
	Nodes        []Node
	Measurements []Measurement
	Backends     []Backend
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
	return c
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
