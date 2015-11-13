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

package framework

import (
	"sync"
)

// slaveRegistry manages node hostnames for slave ids.
type slaveRegistry struct {
	lock        sync.Mutex
	hostNames   map[string]string // by slaveId
	executorIds map[string]string // by slaveId
}

func newSlaveRegistry() *slaveRegistry {
	return &slaveRegistry{
		hostNames:   map[string]string{},
		executorIds: map[string]string{},
	}
}

// Register creates a mapping between a slaveId and slave if not existing.
func (st *slaveRegistry) Register(slaveId, slaveHostname, executorId string) {
	st.lock.Lock()
	defer st.lock.Unlock()

	st.hostNames[slaveId] = slaveHostname
	if executorId != "" {
		st.executorIds[slaveId] = executorId
	}
}

// ExecutorLost removes the executor id for the given slave
func (st *slaveRegistry) ExecutorLost(slaveId string) {
	delete(st.executorIds, slaveId)
}

// SlaveIDs returns the keys of the registry
func (st *slaveRegistry) SlaveIDs() []string {
	st.lock.Lock()
	defer st.lock.Unlock()
	slaveIds := make([]string, 0, len(st.hostNames))
	for slaveID := range st.hostNames {
		slaveIds = append(slaveIds, slaveID)
	}
	return slaveIds
}

// HostName looks up a hostname for a given slaveId
func (st *slaveRegistry) HostName(slaveId string) string {
	st.lock.Lock()
	defer st.lock.Unlock()
	return st.hostNames[slaveId]
}

// ExecutorId looks up an executor id for a given slaveId. An empty string is
// returned if no executor is running on that slave.
func (st *slaveRegistry) ExecutorId(slaveId string) string {
	st.lock.Lock()
	defer st.lock.Unlock()
	return st.executorIds[slaveId]
}
