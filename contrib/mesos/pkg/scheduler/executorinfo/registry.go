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
	"sync"

	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/mesos/mesos-go/mesosproto"
	"k8s.io/kubernetes/contrib/mesos/pkg/node"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/meta"
	client "k8s.io/kubernetes/pkg/client/unversioned"
)

type Registry interface {
	New(nodename string, resources []*mesosproto.Resource) *mesosproto.ExecutorInfo
	Get(nodename, id string) (*mesosproto.ExecutorInfo, error)
}

type registry struct {
	// items stores executor ID keys and ExecutorInfo values
	items map[string]*mesosproto.ExecutorInfo
	// protects fields above
	mu sync.RWMutex

	lookupNode node.LookupFunc
	prototype  *mesosproto.ExecutorInfo
	client     *client.Client
}

func NewRegistry(lookupNode node.LookupFunc, prototype *mesosproto.ExecutorInfo, client *client.Client) Registry {
	return &registry{
		items:      map[string]*mesosproto.ExecutorInfo{},
		lookupNode: lookupNode,
		prototype:  prototype,
	}
}

func (r *registry) New(nodename string, resources []*mesosproto.Resource) *mesosproto.ExecutorInfo {
	e := proto.Clone(r.prototype).(*mesosproto.ExecutorInfo)
	e.Resources = resources

	// hostname needs of the executor needs to match that of the offer, otherwise
	// the kubelet node status checker/updater is very unhappy
	setCommandArgument(e, "--hostname-override", nodename, true)
	id, _ := NewExecutorID(e)
	e.ExecutorId = id

	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.items[id.GetValue()]
	if ok {
		return info
	}

	r.items[id.GetValue()] = e
	return e
}

func (r *registry) Get(nodename, id string) (*mesosproto.ExecutorInfo, error) {
	// first try to read from cached items
	r.mu.RLock()
	info, ok := r.items[id]
	r.mu.RUnlock()

	if ok {
		return info, nil
	}

	result, err := r.getFromNode(nodename, id)
	if err != nil {
		// master claims there is an executor with id, we cannot find any meta info
		// => no way to recover this node
		return nil, fmt.Errorf(
			"failed to recover executor info for node %q, error: %v",
			nodename, err,
		)
	}

	return r.New(nodename, result), nil
}

func (r *registry) getFromNode(nodename, id string) ([]*mesosproto.Resource, error) {
	n := r.lookupNode(nodename)
	if n == nil {
		return nil, fmt.Errorf("nodename %q not found", nodename)
	}

	annotatedId, ok := n.Annotations[meta.ExecutorIdKey]
	if annotatedId != id {
		return nil, fmt.Errorf(
			"want executor id %q but got %q from nodename %q",
			id, annotatedId, nodename,
		)
	}

	encoded, ok := n.Annotations[meta.ExecutorResourcesKey]
	if !ok {
		return nil, fmt.Errorf(
			"no %q annotation found in nodename %q",
			meta.ExecutorResourcesKey, nodename,
		)
	}

	return DecodeResources(strings.NewReader(encoded))
}

func setCommandArgument(ei *mesosproto.ExecutorInfo, flag, value string, create bool) {
	argv := ei.Command.Arguments
	overwrite := false

	for i, arg := range argv {
		if strings.HasPrefix(arg, flag+"=") {
			overwrite = true
			argv[i] = flag + "=" + value
			break
		}
	}

	if !overwrite && create {
		ei.Command.Arguments = append(argv, flag+"="+value)
	}
}
