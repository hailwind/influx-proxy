// Copyright 2016 Eleme. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/hailwind/influx-proxy/backend"
	"github.com/hailwind/influx-proxy/config"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	ErrConfig   = errors.New("config parse error")
	ConfigFile  string
	LogFilePath string
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	flag.StringVar(&LogFilePath, "log-file-path", "", "output file")
	flag.StringVar(&ConfigFile, "config", "", "config file")
	flag.Parse()
}

func initLog() {
	if LogFilePath == "" {
		log.SetOutput(os.Stdout)
	} else {
		log.SetOutput(&lumberjack.Logger{
			Filename:   LogFilePath,
			MaxSize:    100,
			MaxBackups: 5,
			MaxAge:     7,
		})
	}
}

func main() {
	initLog()

	var err error
	var c config.Conf
	conf := c.GetConf(ConfigFile)
	var nodecfg = conf.Node

	ic := backend.NewInfluxCluster(nodecfg, conf.Backends, conf.Measurements)
	ic.LoadConfig()

	mux := http.NewServeMux()
	NewHttpService(ic, nodecfg.DB).Register(mux)

	log.Printf("http service start.")
	server := &http.Server{
		Addr:        nodecfg.ListenAddr,
		Handler:     mux,
		IdleTimeout: time.Duration(nodecfg.IdleTimeout) * time.Second,
	}
	if nodecfg.IdleTimeout <= 0 {
		server.IdleTimeout = 10 * time.Second
	}
	err = server.ListenAndServe()
	if err != nil {
		log.Print(err)
		return
	}
}
