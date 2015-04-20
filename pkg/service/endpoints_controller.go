/*
Copyright 2014 Google Inc. All rights reserved.

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
	"sync"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"

	"github.com/golang/glog"
)

// An EndpointsController knows how to update a client's service
// endpoints for a certain networking strategy (e.g. one ip per pod).
type EndpointsController interface {
	// SyncServiceEndpoints updates the client's service endpoints.
	SyncServiceEndpoints() error
}

// A ControllerFunc creates an EndpointsController that updates the
// given client.
type ControllerFunc func(*client.Client) EndpointsController

var (
	controllersLock sync.Mutex
	controllers     = make(map[string]ControllerFunc)
)

// RegisterEndpointsController registers a ControllerFunc by name.
func RegisterEndpointsController(name string, f ControllerFunc) {
	controllersLock.Lock()
	defer controllersLock.Unlock()
	if _, ok := controllers[name]; ok {
		glog.Fatalf("Endpoints controller %q was registered twice", name)
	}
	glog.V(1).Infof("Registered endpoints controller %q", name)
	controllers[name] = f
}

// GetEndpointsController creates an EndpointsController using the
// ControllerFunc registered to the given name. If there is no
// ControllerFunc registered to the given name, it returns nil.
func GetEndpointsController(name string, client *client.Client) EndpointsController {
	controllersLock.Lock()
	defer controllersLock.Unlock()
	f, ok := controllers[name]
	if !ok {
		return nil
	}
	return f(client)
}
