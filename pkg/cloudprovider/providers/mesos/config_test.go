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
	"bytes"
	"reflect"
	"testing"
	"time"

	log "github.com/golang/glog"
	"os"
)

// test mesos.createDefaultConfig
func Test_createDefaultConfig(t *testing.T) {
	defer log.Flush()

	config := createDefaultConfig()

	if got, want := config.Mesos_Cloud.MesosMaster, DefaultMesosMaster; got != want {
		t.Fatalf("unexpected MesosMaster: got=%q, want=%q", got, want)
	}

	if got, want := config.Mesos_Cloud.MesosHttpClientTimeout.Duration, DefaultHttpClientTimeout; got != want {
		t.Fatalf("unexpected MesosHttpClientTimeout: got=%q, want=%q", got, want)
	}

	if got, want := config.Mesos_Cloud.StateCacheTTL.Duration, DefaultStateCacheTTL; got != want {
		t.Fatalf("unexpected StateCacheTTL: got=%q, want=%q", got, want)
	}

	if got, want := len(config.Mesos_Node.Labels), 0; got != want {
		t.Fatalf("unexpected number of labels: got=%q, want=%q", got, want)
	}

	hostname, _ := os.Hostname()
	if got, want := config.Mesos_Node.Name, hostname; got != want {
		t.Fatalf("unexpected node name: got=%q, want=%q", got, want)
	}
}

// test mesos.readConfig
func Test_readConfig(t *testing.T) {
	defer log.Flush()

	configString := `
[mesos-cloud]
	mesos-master        = leader.mesos:5050
	http-client-timeout = 500ms
	state-cache-ttl     = 1h

[mesos-node]
    label = rack:a
    label = gen:2014
    name = mesosslave1
`

	reader := bytes.NewBufferString(configString)

	config, err := readConfig(reader)
	if err != nil {
		t.Fatalf("Reading configuration yielded an error: %#v", err)
	}

	if got, want := config.Mesos_Cloud.MesosMaster, "leader.mesos:5050"; got != want {
		t.Fatalf("unexpected MesosMaster: got=%q, want=%q", got, want)
	}

	if got, want := config.Mesos_Cloud.MesosHttpClientTimeout.Duration, time.Duration(500)*time.Millisecond; got != want {
		t.Fatalf("unexpected MesosHttpClientTimeout: got=%q, want=%q", got, want)
	}

	if got, want := config.Mesos_Cloud.StateCacheTTL.Duration, time.Duration(1)*time.Hour; got != want {
		t.Fatalf("unexpected StateCacheTTL: got=%q, want=%q", got, want)
	}

	if got, want := config.Mesos_Node.Labels, []Label{{"rack", "a"}, {"gen", "2014"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected labels: got=%q, want=%q", got, want)
	}

	if got, want := config.Mesos_Node.Name, "mesosslave1"; got != want {
		t.Fatalf("unexpected node name: got=%q, want=%q", got, want)
	}
}
