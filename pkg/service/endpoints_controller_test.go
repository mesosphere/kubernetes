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
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
)

type testController struct {
	dummy int // so that memory is actually allocated
}

func (tc *testController) SyncServiceEndpoints() error { return nil }

func TestRegisterAndGet(t *testing.T) {
	var lastController *testController
	RegisterEndpointsController("test", func(c *client.Client) EndpointsController {
		tc := &testController{}
		lastController = tc
		return tc
	})
	ec := GetEndpointsController("test", nil)
	if ec == nil {
		t.Fatal("controller is nil, want non-nil")
	}
	tc, ok := ec.(*testController)
	if !ok {
		t.Fatalf("type of controller is %T, want %T", ec, &testController{})
	}
	if tc != lastController {
		t.Fatalf("got %p, want %p", tc, lastController)
	}
}

func TestMissingControllerFunc(t *testing.T) {
	ec := GetEndpointsController("missing", nil)
	if ec != nil {
		t.Fatal("controller is non-nil, want nil")
	}
}
