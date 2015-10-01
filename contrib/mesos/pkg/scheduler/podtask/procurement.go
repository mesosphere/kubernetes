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

package podtask

import (
	"fmt"
	"math"

	log "github.com/golang/glog"
	mesos "github.com/mesos/mesos-go/mesosproto"
	"github.com/mesos/mesos-go/mesosutil"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/executorinfo"
	mresource "k8s.io/kubernetes/contrib/mesos/pkg/scheduler/resource"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/labels"
)

// NewDefaultProcurement returns the default procurement strategy that combines validation
// and responsible Mesos resource procurement. c and m are resource quantities written into
// k8s api.Pod.Spec's that don't declare resources (all containers in k8s-mesos require cpu
// and memory limits).
func NewDefaultProcurement(prototype *mesos.ExecutorInfo, eir executorinfo.Registry) Procurement {
	return AllOrNothingProcurement([]Procurement{
		NewValidateProcurement(),
		NewNodeProcurement(),
		NewPodResourcesProcurement(),
		NewPortsProcurement(),
		NewExecutorResourceProcurer(prototype, eir),
	})
}

// Procurement funcs allocate resources for a task from an offer.
// Both the offer and the spec may be modified, the task and the node must not.
type ProcurementFunc func(*T, *mesos.Offer, *api.Node, *Spec) error

func (p ProcurementFunc) Procure(t *T, o *mesos.Offer, n *api.Node, s *Spec) error {
	return p(t, o, n, s)
}

type Procurement interface {
	Procure(*T, *mesos.Offer, *api.Node, *Spec) error
}

// AllOrNothingProcurement provides a convenient wrapper around multiple Procurement
// objectives: the failure of any Procurement in the set results in Procure failing.
// see AllOrNothingProcurement.Procure
type AllOrNothingProcurement []Procurement

// Procure runs each Procurement in the receiver list. The first Procurement func that
// fails triggers T.Reset() and the error is returned, otherwise returns nil.
func (a AllOrNothingProcurement) Procure(t *T, offer *mesos.Offer, n *api.Node, spec *Spec) error {
	for _, p := range a {
		err := p.Procure(t, offer, n, spec)
		if err != nil {
			return err
		}
	}
	return nil
}

// NewValidateProcurement checks that the offered resources are kosher, and if not panics.
// If things check out ok, t.Spec is cleared and nil is returned.
func NewValidateProcurement() Procurement {
	return ProcurementFunc(func(t *T, offer *mesos.Offer, _ *api.Node, spec *Spec) error {
		if offer == nil {
			//programming error
			panic("offer details are nil")
		}
		return nil
	})
}

// NewNodeProcurement returns a Procurement updating t.Spec in preparation for the task to be launched on the
// slave associated with the offer.
func NewNodeProcurement() Procurement {
	return ProcurementFunc(func(t *T, offer *mesos.Offer, n *api.Node, spec *Spec) error {
		// if the user has specified a target host, make sure this offer is for that host
		if t.Pod.Spec.NodeName != "" && offer.GetHostname() != t.Pod.Spec.NodeName {
			return fmt.Errorf(
				"NodeName %q does not match offer hostname %q",
				t.Pod.Spec.NodeName, offer.GetHostname(),
			)
		}

		// check the NodeSelector
		if len(t.Pod.Spec.NodeSelector) > 0 {
			if n.Labels == nil {
				return fmt.Errorf(
					"NodeSelector %v does not match empty labels of pod %s/%s",
					t.Pod.Spec.NodeSelector, t.Pod.Namespace, t.Pod.Name,
				)
			}
			selector := labels.SelectorFromSet(t.Pod.Spec.NodeSelector)
			if !selector.Matches(labels.Set(n.Labels)) {
				return fmt.Errorf(
					"NodeSelector %v does not match labels %v of pod %s/%s",
					t.Pod.Spec.NodeSelector, t.Pod.Labels, t.Pod.Namespace, t.Pod.Name,
				)
			}
		}

		spec.SlaveID = offer.GetSlaveId().GetValue()
		spec.AssignedSlave = offer.GetHostname()

		return nil
	})
}

// NewPodResourcesProcurement converts k8s pod cpu and memory resource requirements into
// mesos resource allocations.
func NewPodResourcesProcurement() Procurement {
	return ProcurementFunc(func(t *T, offer *mesos.Offer, _ *api.Node, spec *Spec) error {
		// TODO(sttts): fall back to requested resources if resource limit cannot be fulfilled by the offer
		_, limits, err := api.PodRequestsAndLimits(&t.Pod)
		if err != nil {
			return err
		}

		wantedCpus := float64(mresource.NewCPUShares(limits[api.ResourceCPU]))
		wantedMem := float64(mresource.NewMegaBytes(limits[api.ResourceMemory]))

		log.V(4).Infof(
			"trying to match offer with pod %v/%v: cpus: %.2f mem: %.2f MB",
			t.Pod.Namespace, t.Pod.Name, wantedCpus, wantedMem,
		)

		podRoles := t.Roles()
		procuredCpu, remaining := procureRoleResources("cpus", wantedCpus, podRoles, offer.GetResources())
		if procuredCpu == nil {
			return fmt.Errorf(
				"not enough cpu resources for pod %s/%s: want=%v",
				t.Pod.Namespace, t.Pod.Name, wantedCpus,
			)
		}

		procuredMem, remaining := procureRoleResources("mem", wantedMem, podRoles, remaining)
		if procuredMem == nil {
			return fmt.Errorf(
				"not enough mem resources for pod %s/%s: want=%v",
				t.Pod.Namespace, t.Pod.Name, wantedMem,
			)
		}

		spec.Resources = append(spec.Resources, append(procuredCpu, procuredMem...)...)
		return nil
	})
}

// NewPortsProcurement returns a Procurement procuring ports
func NewPortsProcurement() Procurement {
	return ProcurementFunc(func(t *T, offer *mesos.Offer, _ *api.Node, spec *Spec) error {
		// fill in port mapping
		if mapping, err := t.mapper.Map(t, offer); err != nil {
			return err
		} else {
			ports := []Port{}
			for _, entry := range mapping {
				ports = append(ports, Port{
					Port: entry.OfferPort,
					Role: entry.Role,
				})
			}
			spec.PortMap = mapping
			spec.Resources = append(spec.Resources, portRangeResources(ports)...)
		}
		return nil
	})
}

// NewExecutorResourceProcurer returns a Procurement procuring executor resources
// If a given offer has no executor IDs set, the given prototype executor resources are considered for procurement.
// If a given offer has one executor ID set, only pod resources are being procured.
// An offer with more than one executor ID implies an invariant violation and the first executor ID is being considered.
func NewExecutorResourceProcurer(prototype *mesos.ExecutorInfo, registry executorinfo.Registry) Procurement {
	return ProcurementFunc(func(t *T, offer *mesos.Offer, _ *api.Node, spec *Spec) error {
		eids := len(offer.GetExecutorIds())
		switch {
		case eids == 0:
			wantedCpus := sumResources(filterResources(prototype.GetResources(), isScalar, hasName("cpus")))
			wantedMem := sumResources(filterResources(prototype.GetResources(), isScalar, hasName("mem")))

			procuredCpu, remaining := procureRoleResources("cpus", wantedCpus, t.allowedRoles, offer.GetResources())
			if procuredCpu == nil {
				return fmt.Errorf("not enough cpu resources for executor: want=%v", wantedCpus)
			}

			procuredMem, remaining := procureRoleResources("mem", wantedMem, t.allowedRoles, remaining)
			if procuredMem == nil {
				return fmt.Errorf("not enough mem resources for executor: want=%v", wantedMem)
			}

			offer.Resources = remaining
			spec.Executor = registry.New(offer.GetHostname(), append(procuredCpu, procuredMem...))
			return nil

		case eids > 1:
			log.Errorf("got %d executor IDs in offer, want 1 (procuring first)", eids)
			fallthrough

		default: // eids >= 1
			// read 1st executor id executor info from cache/attributes/...
			eid := offer.GetExecutorIds()[0].GetValue()
			log.V(5).Infof("found executor id %q", eid)

			e, err := registry.Get(offer.GetHostname(), eid)
			if err != nil {
				return err
			}

			spec.Executor = e
			return nil
		}
	})
}

// smallest number such that 1.0 + epsilon != 1.0
// see https://github.com/golang/go/issues/966
var epsilon = math.Nextafter(1, 2) - 1

// procureRoleResources procures offered resources that
// 1. Match the given name
// 2. Match the given roles
// 3. The given wanted scalar value can be fully consumed by offered resources
// Roles are being considered in the specified roles slice ordering.
func procureRoleResources(name string, want float64, roles []string, offered []*mesos.Resource) (procured, remaining []*mesos.Resource) {
	sorted := byRoles(roles...).sort(offered)
	procured = make([]*mesos.Resource, 0, len(sorted))
	remaining = make([]*mesos.Resource, 0, len(sorted))

	for _, r := range sorted {
		if want >= epsilon && resourceMatchesAll(r, hasName(name), isScalar) {
			left, role := r.GetScalar().GetValue(), r.Role
			consumed := math.Min(want, left)

			want -= consumed
			left -= consumed

			if left >= epsilon {
				r = mesosutil.NewScalarResource(name, left)
				r.Role = role
				remaining = append(remaining, r)
			}

			consumedRes := mesosutil.NewScalarResource(name, consumed)
			consumedRes.Role = role
			procured = append(procured, consumedRes)
		} else {
			remaining = append(remaining, r)
		}
	}

	// demanded value (want) was not fully consumed violating invariant 3.
	// thus no resources must be procured
	if want >= epsilon {
		return nil, offered
	}

	return
}
