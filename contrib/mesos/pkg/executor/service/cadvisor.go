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

package service

import (
	sservice "k8s.io/kubernetes/contrib/mesos/pkg/scheduler/service"
	"k8s.io/kubernetes/pkg/kubelet/cadvisor"

	cadvisorApi "github.com/google/cadvisor/info/v1"
	"github.com/mesos/mesos-go/mesosproto"
)

type MesosCadvisor struct {
	cadvisor.Interface
	slaveInfo *mesosproto.SlaveInfo
}

func NewMesosCadvisor(si *mesosproto.SlaveInfo, port uint) (*MesosCadvisor, error) {
	c, err := cadvisor.New(port)
	if err != nil {
		return nil, err
	}
	return &MesosCadvisor{c, si}, nil
}

func (mc *MesosCadvisor) MachineInfo() (*cadvisorApi.MachineInfo, error) {
	mi, err := mc.Interface.MachineInfo()
	if err != nil {
		return nil, err
	}
	mesosMi := *mi

	for _, r := range mc.slaveInfo.GetResources() {
		if r == nil || r.GetType() != mesosproto.Value_SCALAR {
			continue
		}

		// TODO(sttts): use custom executor CPU and mem values here, not the defaults
		switch r.GetName() {
		case "cpus":
			mesosMi.NumCores = int(r.GetScalar().GetValue() - float64(sservice.DefaultExecutorCPUs))
		case "mem":
			mesosMi.MemoryCapacity = int64(r.GetScalar().GetValue()-float64(sservice.DefaultExecutorMem)) * 1024 * 1024
		}
	}

	return &mesosMi, nil
}
