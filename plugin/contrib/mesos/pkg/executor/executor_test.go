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
	"sync/atomic"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/testapi"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubelet/dockertools"
	"github.com/GoogleCloudPlatform/kubernetes/plugin/contrib/mesos/pkg/scheduler/podtask"
	"github.com/golang/glog"
	bindings "github.com/mesos/mesos-go/executor"
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var test_v = flag.Int("test-v", 0, "test -v")

type suicideTracker struct {
	suicideWatcher
	stops  uint32
	resets uint32
	timers uint32
	jumps  *uint32
}

func (t *suicideTracker) Reset(d time.Duration) bool {
	defer func() { t.resets++ }()
	return t.suicideWatcher.Reset(d)
}

func (t *suicideTracker) Stop() bool {
	defer func() { t.stops++ }()
	return t.suicideWatcher.Stop()
}

func (t *suicideTracker) Next(d time.Duration, driver bindings.ExecutorDriver, f jumper) suicideWatcher {
	tracker := &suicideTracker{
		stops:  t.stops,
		resets: t.resets,
		jumps:  t.jumps,
		timers: t.timers + 1,
	}
	jumper := tracker.makeJumper(f)
	tracker.suicideWatcher = t.suicideWatcher.Next(d, driver, jumper)
	return tracker
}

func (t *suicideTracker) makeJumper(_ jumper) jumper {
	return jumper(func(driver bindings.ExecutorDriver, cancel <-chan struct{}) {
		glog.Warningln("jumping?!")
		if t.jumps != nil {
			atomic.AddUint32(t.jumps, 1)
		}
	})
}

func TestSuicide_zeroTimeout(t *testing.T) {
	defer glog.Flush()

	k := New(Config{})
	tracker := &suicideTracker{suicideWatcher: k.suicideWatch}
	k.suicideWatch = tracker

	ch := k.resetSuicideWatch(nil)

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for reset of suicide watch")
	}
	if tracker.stops != 0 {
		t.Fatalf("expected no stops since suicideWatchTimeout was never set")
	}
	if tracker.resets != 0 {
		t.Fatalf("expected no resets since suicideWatchTimeout was never set")
	}
	if tracker.timers != 0 {
		t.Fatalf("expected no timers since suicideWatchTimeout was never set")
	}
}

func TestSuicide_WithTasks(t *testing.T) {
	defer glog.Flush()

	k := New(Config{
		SuicideTimeout: 50 * time.Millisecond,
	})

	jumps := uint32(0)
	tracker := &suicideTracker{suicideWatcher: k.suicideWatch, jumps: &jumps}
	k.suicideWatch = tracker

	k.tasks["foo"] = &kuberTask{} // prevent suicide attempts from succeeding

	// call reset with a nil timer
	glog.Infoln("resetting suicide watch with 1 task")
	select {
	case <-k.resetSuicideWatch(nil):
		tracker = k.suicideWatch.(*suicideTracker)
		if tracker.stops != 1 {
			t.Fatalf("expected suicide attempt to Stop() since there are registered tasks")
		}
		if tracker.resets != 0 {
			t.Fatalf("expected no resets since")
		}
		if tracker.timers != 0 {
			t.Fatalf("expected no timers since")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("initial suicide watch setup failed")
	}

	delete(k.tasks, "foo") // zero remaining tasks
	k.suicideTimeout = 1500 * time.Millisecond
	suicideStart := time.Now()

	// reset the suicide watch, which should actually start a timer now
	glog.Infoln("resetting suicide watch with 0 tasks")
	select {
	case <-k.resetSuicideWatch(nil):
		tracker = k.suicideWatch.(*suicideTracker)
		if tracker.stops != 1 {
			t.Fatalf("did not expect suicide attempt to Stop() since there are no registered tasks")
		}
		if tracker.resets != 1 {
			t.Fatalf("expected 1 resets instead of %d", tracker.resets)
		}
		if tracker.timers != 1 {
			t.Fatalf("expected 1 timers instead of %d", tracker.timers)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("2nd suicide watch setup failed")
	}

	k.lock.Lock()
	k.tasks["foo"] = &kuberTask{} // prevent suicide attempts from succeeding
	k.lock.Unlock()

	// reset the suicide watch, which should stop the existing timer
	glog.Infoln("resetting suicide watch with 1 task")
	select {
	case <-k.resetSuicideWatch(nil):
		tracker = k.suicideWatch.(*suicideTracker)
		if tracker.stops != 2 {
			t.Fatalf("expected 2 stops instead of %d since there are registered tasks", tracker.stops)
		}
		if tracker.resets != 1 {
			t.Fatalf("expected 1 resets instead of %d", tracker.resets)
		}
		if tracker.timers != 1 {
			t.Fatalf("expected 1 timers instead of %d", tracker.timers)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("3rd suicide watch setup failed")
	}

	k.lock.Lock()
	delete(k.tasks, "foo") // allow suicide attempts to schedule
	k.lock.Unlock()

	// reset the suicide watch, which should reset a stopped timer
	glog.Infoln("resetting suicide watch with 0 tasks")
	select {
	case <-k.resetSuicideWatch(nil):
		tracker = k.suicideWatch.(*suicideTracker)
		if tracker.stops != 2 {
			t.Fatalf("expected 2 stops instead of %d since there are no registered tasks", tracker.stops)
		}
		if tracker.resets != 2 {
			t.Fatalf("expected 2 resets instead of %d", tracker.resets)
		}
		if tracker.timers != 1 {
			t.Fatalf("expected 1 timers instead of %d", tracker.timers)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("4th suicide watch setup failed")
	}

	sinceWatch := time.Since(suicideStart)
	time.Sleep(3*time.Second - sinceWatch) // give the first timer to misfire (it shouldn't since Stop() was called)

	if j := atomic.LoadUint32(&jumps); j != 1 {
		t.Fatalf("expected 1 jumps instead of %d since stop was called", j)
	} else {
		glog.Infoln("jumps verified") // glog so we get a timestamp
	}
}

func TestExecutorRegister(t *testing.T) {
	flag.Lookup("v").Value.Set(fmt.Sprint(*test_v))

	mockDriver := MockExecutorDriver{}
	executor := NewTestKubernetesExecutor()

	executor.Init(mockDriver)
	executor.Registered(mockDriver, nil, nil, nil)

	assert.Equal(t, executor.isConnected(), true, "executor should be connected")
	mockDriver.AssertExpectations(t)
}

func TestExecutorReregister(t *testing.T) {
	flag.Lookup("v").Value.Set(fmt.Sprint(*test_v))

	mockDriver := MockExecutorDriver{}
	executor := NewTestKubernetesExecutor()

	executor.Init(mockDriver)
	executor.Registered(mockDriver, nil, nil, nil)

	assert.Equal(t, executor.isConnected(), true, "executor should be connected")
	mockDriver.AssertExpectations(t)
}

func TestExecutorDisconnect(t *testing.T) {
	flag.Lookup("v").Value.Set(fmt.Sprint(*test_v))

	mockDriver := MockExecutorDriver{}
	executor := NewTestKubernetesExecutor()

	executor.Init(mockDriver)
	executor.Registered(mockDriver, nil, nil, nil)
	executor.Disconnected(mockDriver)

	assert.Equal(t, executor.isConnected(), false, "executor should not be connected after Disconnected")
	mockDriver.AssertExpectations(t)
}

func TestExecutorLaunchAndKillTask(t *testing.T) {
	flag.Lookup("v").Value.Set(fmt.Sprint(*test_v))

	mockDriver := MockExecutorDriver{}
	config := Config{
		Docker:    dockertools.ConnectToDockerOrDie("fake://"),
		Updates:   make(chan interface{}, 1024),
		APIClient: client.NewOrDie(&client.Config{Host: "fakehost", Version: testapi.Version()}),
	}
	executor := New(config)

	executor.Init(mockDriver)
	executor.Registered(mockDriver, nil, nil, nil)

	pod := newTestPod(1)
	podTask, err := podtask.New(api.NewDefaultContext(), "",
		*pod, &mesosproto.ExecutorInfo{})
	assert.Equal(t, err, nil, "must be able to create a task from a pod")

	taskInfo := podTask.BuildTaskInfo()
	data, err := api.Codec.Encode(pod)
	assert.Equal(t, err, nil, "must be able to encode a pod's spec data")
	taskInfo.Data = data

	mockDriver.On("SendStatusUpdate", mock.AnythingOfType("*mesosproto.TaskStatus")).Return(mesosproto.Status_DRIVER_RUNNING, nil)
	executor.LaunchTask(mockDriver, taskInfo)
	assert.Equal(t, len(executor.tasks), 1, "executor must be able to create a task")

	mockDriver.On("SendStatusUpdate", mock.AnythingOfType("*mesosproto.TaskStatus")).Return(mesosproto.Status_DRIVER_RUNNING, nil)
	executor.KillTask(mockDriver, taskInfo.TaskId)
	assert.Equal(t, len(executor.tasks), 0, "executor must be able to kill a created task")
	mockDriver.AssertExpectations(t)
}

func TestExecutorFrameworkMessage(t *testing.T) {
	flag.Lookup("v").Value.Set(fmt.Sprint(*test_v))

	mockDriver := MockExecutorDriver{}
	executor := NewTestKubernetesExecutor()

	executor.Init(mockDriver)
	executor.Registered(mockDriver, nil, nil, nil)
	executor.FrameworkMessage(mockDriver, "test framework message")

	// TODO(tyler) make this test more meaningful
	mockDriver.AssertExpectations(t)
}

// Create a pod with a given index, requiring one port
func newTestPod(i int) *api.Pod {
	name := fmt.Sprintf("pod%d", i)
	return &api.Pod{
		TypeMeta: api.TypeMeta{APIVersion: testapi.Version()},
		ObjectMeta: api.ObjectMeta{
			Name:      name,
			Namespace: "default",
			SelfLink:  fmt.Sprintf("http://1.2.3.4/api/v1beta1/pods/%v", i),
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Ports: []api.ContainerPort{
						{
							ContainerPort: 8000 + i,
							Protocol:      api.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: api.PodStatus{
			PodIP: fmt.Sprintf("1.2.3.%d", 4+i),
			Conditions: []api.PodCondition{
				{
					Type:   api.PodReady,
					Status: api.ConditionTrue,
				},
			},
		},
	}
}

func TestExecutorShutdown(t *testing.T) {
	flag.Lookup("v").Value.Set(fmt.Sprint(*test_v))

	mockDriver := MockExecutorDriver{}
	kubeletFinished := make(chan struct{})
	config := Config{
		Docker:  dockertools.ConnectToDockerOrDie("fake://"),
		Updates: make(chan interface{}, 1024),
		ShutdownAlert: func() {
			close(kubeletFinished)
		},
		KubeletFinished: kubeletFinished,
	}
	executor := New(config)

	executor.Init(mockDriver)
	executor.Registered(mockDriver, nil, nil, nil)

	mockDriver.On("Stop").Return(mesosproto.Status_DRIVER_STOPPED, nil).Once()

	executor.Shutdown(mockDriver)

	assert.Equal(t, executor.isConnected(), false, "executor should not be connected after Shutdown")
	assert.Equal(t, executor.isDone(), true, "executor should be in Done state after Shutdown")

	doneChanWorked := false
	select {
	case _, ok := <-executor.done:
		if !ok {
			doneChanWorked = true
		}
	default:
	}
	assert.Equal(t, doneChanWorked, true, "done channel should be closed after shutdown")

	mockDriver.AssertExpectations(t)
}
