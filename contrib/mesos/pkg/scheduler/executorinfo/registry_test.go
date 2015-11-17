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

package executorinfo

import (
	"bytes"
	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/mesosproto"
	"github.com/mesos/mesos-go/mesosutil"
	"k8s.io/kubernetes/contrib/mesos/pkg/node"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/meta"
	"k8s.io/kubernetes/pkg/api"
	"reflect"
	"testing"
)

func TestRegistryGet(t *testing.T) {
	var lookupFunc func() *api.Node
	lookupNode := node.LookupFunc(func(hostname string) *api.Node {
		return lookupFunc()
	})

	prototype := &mesosproto.ExecutorInfo{
		Resources: []*mesosproto.Resource{
			scalar("foo", 1.0, "role1"),
		},
	}
	r, err := NewRegistry(lookupNode, prototype)
	if err != nil {
		t.Error(err)
		return
	}

	var resources bytes.Buffer
	EncodeResources(&resources, prototype.GetResources())

	for i, tt := range []struct {
		apiNode *api.Node
		wantErr bool
	}{
		{
			apiNode: nil,
			wantErr: true,
		}, {
			apiNode: &api.Node{},
			wantErr: true,
		}, {
			apiNode: &api.Node{
				ObjectMeta: api.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantErr: true,
		}, {
			apiNode: &api.Node{
				ObjectMeta: api.ObjectMeta{
					Annotations: map[string]string{
						meta.ExecutorResourcesKey: resources.String(),
					},
				},
			},
			wantErr: false,
		},
	} {
		lookupFunc = func() *api.Node { return tt.apiNode }
		_, err := r.Get("")

		if !tt.wantErr && err != nil {
			t.Errorf("test %d: want error but got none", i)
		}
	}
}

func TestRegistryNew(t *testing.T) {
	for i, tt := range []struct {
		prototype *mesosproto.ExecutorInfo
		resources []*mesosproto.Resource
		want      *mesosproto.ExecutorInfo
	}{
		{
			prototype: &mesosproto.ExecutorInfo{
				ExecutorId: mesosutil.NewExecutorID("exec-id"),
			},
			resources: nil,
			want: &mesosproto.ExecutorInfo{
				ExecutorId: mesosutil.NewExecutorID("exec-id"),
			},
		}, {
			prototype: &mesosproto.ExecutorInfo{
				ExecutorId: mesosutil.NewExecutorID("exec-id"),
			},
			resources: []*mesosproto.Resource{},
			want: &mesosproto.ExecutorInfo{
				ExecutorId: mesosutil.NewExecutorID("exec-id"),
				Resources:  []*mesosproto.Resource{},
			},
		}, {
			prototype: &mesosproto.ExecutorInfo{
				ExecutorId: mesosutil.NewExecutorID("exec-id"),
				Name:       proto.String("foo"),
			},

			resources: []*mesosproto.Resource{
				scalar("foo", 1.0, "role1"),
				scalar("bar", 2.0, "role2"),
			},

			want: &mesosproto.ExecutorInfo{
				ExecutorId: mesosutil.NewExecutorID("exec-id"),
				Name:       proto.String("foo"),
				Resources: []*mesosproto.Resource{
					scalar("foo", 1.0, "role1"),
					scalar("bar", 2.0, "role2"),
				},
			},
		},
	} {
		lookupNode := node.LookupFunc(func(string) *api.Node {
			return nil
		})
		r, err := NewRegistry(lookupNode, tt.prototype)
		if err != nil {
			t.Error(err)
			continue
		}

		got := r.New("", tt.resources)

		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("test #%d\ngot  %v\nwant %v", i, got, tt.want)
		}
	}
}

func TestRegistryNewDup(t *testing.T) {
	lookupNode := node.LookupFunc(func(string) *api.Node {
		return nil
	})

	r, err := NewRegistry(lookupNode, &mesosproto.ExecutorInfo{})
	if err != nil {
		t.Error(err)
		return
	}

	new := r.New("", nil)
	dup := r.New("", nil)

	if !reflect.DeepEqual(new, dup) {
		t.Errorf(
			"expected new and duplicate executorinfo to be the same, but got new %v dup %v",
			new, dup,
		)
	}
}
