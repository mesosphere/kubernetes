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

package e2e

import (
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/kubernetes/pkg/fields"
)

var _ = Describe("Mesos", func() {
	framework := NewFramework("pods")

	BeforeEach(func() {
		SkipUnlessProviderIs("mesos/docker")
	})

	AfterEach(func() {
	})

	// Test Nodes does not have any label, hence it should be impossible to schedule Pod with
	// nonempty Selector set.
	It("matches NodeSelector with Mesos slave attributes.", func() {
		podClient := framework.Client.Pods(framework.Namespace.Name)
		nodeClient := framework.Client.Nodes()

		By("Trying to schedule Pod with rack:a NodeSelector.")
		name := "rack-a-pod-" + string(util.NewUUID())
		pod := &api.Pod{
			TypeMeta: api.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: api.ObjectMeta{
				Name:   name,
			},
			Spec: api.PodSpec{
				Containers: []api.Container{
					{
						Name:  name,
						Image: "gcr.io/google_containers/pause:go",
					},
				},
				NodeSelector: map[string]string{
					"rack": "a",
				},
			},
		}

		defer podClient.Delete(pod.Name, nil)
		_, err := podClient.Create(pod)
		if err != nil {
			Failf("Error creating a pod: %v", err)
		}
		expectNoError(framework.WaitForPodRunning(pod.Name))

		startedPod, err := podClient.Get(name)
		if err != nil {
			Failf("Failed to query for pods: %v", err)
		}

		rackA := labels.SelectorFromSet(pod.Spec.NodeSelector)
		nodes, err := nodeClient.List(rackA, fields.Everything())
		if err != nil {
			Failf("Failed to query for node: %v", err)
		}
		Expect(len(nodes.Items)).To(Equal(1))

		var addr string
		for _, a := range nodes.Items[0].Status.Addresses {
			if a.Type == api.NodeInternalIP {
				addr = a.Address
			}
		}
		Expect(len(addr)).NotTo(Equal(""))

		Expect(startedPod.Spec.NodeName).To(Equal(addr))
	})
})
