/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mesos

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/scalingdata/gcfg"
)

const (
	DefaultMesosMaster       = "localhost:5050"
	DefaultHttpClientTimeout = time.Duration(10) * time.Second
	DefaultStateCacheTTL     = time.Duration(5) * time.Second
)

// Example Mesos cloud provider configuration file:
//
// [mesos-cloud]
//  mesos-master        = leader.mesos:5050
//	http-client-timeout = 500ms
//	state-cache-ttl     = 1h
//
// [mesos-node]
//  labels = {"rack": "a", "gen": "2014"}

type Config struct {
	Mesos_Cloud Cloud
	Mesos_Node Node
}

type Cloud struct {
	MesosMaster            string   `gcfg:"mesos-master"`
	MesosHttpClientTimeout Duration `gcfg:"http-client-timeout"`
	StateCacheTTL          Duration `gcfg:"state-cache-ttl"`
}

type Label struct {
	key, value string
}

type Node struct {
	Labels []Label `gcfg:"label"`
	Name   string  `gcfg:"name"`
}

type Duration struct {
	Duration time.Duration `gcfg:"duration"`
}

func (d *Duration) UnmarshalText(data []byte) error {
	underlying, err := time.ParseDuration(string(data))
	if err == nil {
		d.Duration = underlying
	}
	return err
}

func (l *Label) UnmarshalText(data []byte) error {
	s := string(data)
	cs := strings.SplitN(s, ":", 2)
	l.key = cs[0]
	if len(cs) == 2 {
		l.value = cs[1]
	}
	return nil
}

func createDefaultConfig() *Config {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	return &Config{
		Mesos_Cloud: Cloud{
			MesosMaster:            DefaultMesosMaster,
			MesosHttpClientTimeout: Duration{Duration: DefaultHttpClientTimeout},
			StateCacheTTL:          Duration{Duration: DefaultStateCacheTTL},
		},
		Mesos_Node: Node{
			Labels: []Label{},
			Name: hostname,
		},
	}
}

func readConfig(configReader io.Reader) (*Config, error) {
	config := createDefaultConfig()
	if configReader != nil {
		if err := gcfg.ReadInto(config, configReader); err != nil {
			return nil, err
		}
	}
	return config, nil
}
