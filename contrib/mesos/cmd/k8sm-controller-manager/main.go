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

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/cloudprovider/mesos"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/healthz"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/version/verflag"

	"github.com/GoogleCloudPlatform/kubernetes/contrib/mesos/pkg/controllermanager"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
)

func init() {
	healthz.DefaultHealthz()
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	s := controllermanager.NewCMServer()
	s.AddFlags(pflag.CommandLine)

	util.InitFlags()
	util.InitLogs()
	defer util.FlushLogs()

	verflag.PrintAndExitIfRequested()

	if s.CloudProvider != mesos.ProviderName {
		glog.Fatalf("Only provider %v is supported, you specified %v", mesos.ProviderName, s.CloudProvider)
	}

	if err := s.Run(pflag.CommandLine.Args()); err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
}
