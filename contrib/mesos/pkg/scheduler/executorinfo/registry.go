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
)

// Registry is the interface that provides methods for interacting
// with a registry of executorinfo objects
type Registry interface {
	// New returns an executor info object based on a given hostname and resources
	New(hostname string, resources []*mesosproto.Resource) *mesosproto.ExecutorInfo
	// Get looks up an executorinfo object for the given hostname and executorinfo ID.
	Get(hostname, id string) (*mesosproto.ExecutorInfo, error)
}

// registry implements a map-based in-memory executorinfo registry
type registry struct {
	// items stores executor ID keys and ExecutorInfo values
	items map[string]*mesosproto.ExecutorInfo
	// protects fields above
	mu sync.RWMutex

	lookupNode node.LookupFunc
	prototype  *mesosproto.ExecutorInfo
}

// NewRegistry returns a new executorinfo registry.
// The given prototype is being used for properties other than resources.
func NewRegistry(lookupNode node.LookupFunc, prototype *mesosproto.ExecutorInfo) (Registry, error) {
	if prototype == nil {
		return nil, fmt.Errorf("no prototype given")
	}

	if lookupNode == nil {
		return nil, fmt.Errorf("no lookupNode given")
	}

	return &registry{
		items:      map[string]*mesosproto.ExecutorInfo{},
		lookupNode: lookupNode,
		prototype:  prototype,
	}, nil
}

func (r *registry) New(hostname string, resources []*mesosproto.Resource) *mesosproto.ExecutorInfo {
	e := proto.Clone(r.prototype).(*mesosproto.ExecutorInfo)
	e.Resources = resources

	// hostname needs of the executor needs to match that of the offer, otherwise
	// the kubelet node status checker/updater is very unhappy
	setCommandArgument(e, "--hostname-override", hostname)
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

func (r *registry) Get(hostname, id string) (*mesosproto.ExecutorInfo, error) {
	// first try to read from cached items
	r.mu.RLock()
	info, ok := r.items[id]
	r.mu.RUnlock()

	if ok {
		return info, nil
	}

	result, err := r.getFromNode(hostname, id)
	if err != nil {
		// master claims there is an executor with id, we cannot find any meta info
		// => no way to recover this node
		return nil, fmt.Errorf(
			"failed to recover executor info for node %q, error: %v",
			hostname, err,
		)
	}

	ei := r.New(hostname, result)

	// check that the newly created ExecutorInfo is compatible with the
	// running executor on the node. If the ids differ, this means that the
	// ExecutorInfos are different and we don't support launching a new task
	// through them. This can happen either on configuration upgrade leading to
	// different executor command line, or on a different k8sm version with
	// different executor settings.
	if ei.GetExecutorId().GetValue() != id {
		return nil, fmt.Errorf(
			"executor on node %q with id %q does not match the current configuration or version which would result in id %q",
			hostname, ei.GetExecutorId().GetValue(), id,
		)
	}

	return ei, nil
}

// getFromNode looks up executorinfo resources for the given hostname and executorinfo ID
// or returns an error in case of failure.
func (r *registry) getFromNode(hostname, id string) ([]*mesosproto.Resource, error) {
	n := r.lookupNode(hostname)
	if n == nil {
		return nil, fmt.Errorf("hostname %q not found", hostname)
	}

	annotatedId, ok := n.Annotations[meta.ExecutorIdKey]
	if !ok {
		return nil, fmt.Errorf(
			"no %q annotation found in hostname %q",
			meta.ExecutorIdKey, hostname,
		)
	}

	if annotatedId != id {
		return nil, fmt.Errorf(
			"want executor id %q but got %q from nodename %q",
			id, annotatedId, hostname,
		)
	}

	encoded, ok := n.Annotations[meta.ExecutorResourcesKey]
	if !ok {
		return nil, fmt.Errorf(
			"no %q annotation found in hostname %q",
			meta.ExecutorResourcesKey, hostname,
		)
	}

	return DecodeResources(strings.NewReader(encoded))
}

// setCommandArgument sets the given flag to the given value
// in the command arguments of the given executoringfo.
func setCommandArgument(ei *mesosproto.ExecutorInfo, flag, value string) {
	if ei.Command == nil {
		return
	}

	argv := ei.Command.Arguments
	overwrite := false

	for i, arg := range argv {
		if strings.HasPrefix(arg, flag+"=") {
			overwrite = true
			argv[i] = flag + "=" + value
			break
		}
	}

	if !overwrite {
		ei.Command.Arguments = append(argv, flag+"="+value)
	}
}
