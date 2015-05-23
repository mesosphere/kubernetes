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

package executor

import (
	"flag"
	"fmt"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubelet/dockertools"
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockExecutorDriver struct {
	mock.Mock
}

func (m MockExecutorDriver) Start() (mesosproto.Status, error) {
	args := m.Called()
	return status(args, 0), args.Error(1)
}

func (m MockExecutorDriver) Stop() (mesosproto.Status, error) {
	args := m.Called()
	return status(args, 0), args.Error(1)
}

func (m MockExecutorDriver) Abort() (mesosproto.Status, error) {
	args := m.Called()
	return status(args, 0), args.Error(1)
}

func (m MockExecutorDriver) Join() (mesosproto.Status, error) {
	args := m.Called()
	return status(args, 0), args.Error(1)
}

func (m MockExecutorDriver) Run() (mesosproto.Status, error) {
	args := m.Called()
	return status(args, 0), args.Error(1)
}

func (m MockExecutorDriver) SendStatusUpdate(taskStatus *mesosproto.TaskStatus) (mesosproto.Status, error) {
	args := m.Called(taskStatus)
	return status(args, 0), args.Error(1)
}

func (m MockExecutorDriver) SendFrameworkMessage(msg string) (mesosproto.Status, error) {
	args := m.Called(m)
	return status(args, 0), args.Error(1)
}

func status(args mock.Arguments, at int) (val mesosproto.Status) {
	if x := args.Get(at); x != nil {
		val = x.(mesosproto.Status)
	}
	return
}

func NewTestKubernetesExecutor() *KubernetesExecutor {
	return New(Config{
		Docker:  dockertools.ConnectToDockerOrDie("fake://"),
		Updates: make(chan interface{}, 1024),
	})
}

func TestExecutorNew(t *testing.T) {
	flag.Lookup("v").Value.Set(fmt.Sprint(*test_v))

	mockDriver := MockExecutorDriver{}
	executor := NewTestKubernetesExecutor()

	assert.Equal(t, executor.isConnected(), false, "executor should not be connected on initialization")
	assert.NotNil(t, executor.isConnected(), "executor should not be nil")
	mockDriver.AssertExpectations(t)
}
