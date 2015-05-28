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
	"github.com/GoogleCloudPlatform/kubernetes/plugin/contrib/mesos/pkg/controllermanager"
)

// NewHyperkubeServer creates a new hyperkube Server object that includes the
// description and flags.
func NewControllerManager() *Server {
	s := controllermanager.NewCMServer()

	hks := Server{
		SimpleUsage: "controller-manager",
		Long:        "A server that runs a set of active components. This includes replication controllers, service endpoints and nodes.",
		Run: func(_ *Server, args []string) error {
			return s.Run(args)
		},
	}
	s.AddFlags(hks.Flags())
	return &hks
}
