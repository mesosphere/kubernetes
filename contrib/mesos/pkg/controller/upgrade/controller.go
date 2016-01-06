package upgrade

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"

	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/meta"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/client/record"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/controller/node"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util"
)

var (
	nodeResyncPeriod = 5 * time.Minute
	nodeCleanPeriod  = 30 * time.Second

	// controls how often NodeController will try to evict Pods from non-responsive Nodes.
	nodeEvictionPeriod = 100 * time.Millisecond

	maximumGracePeriod = 5 * time.Minute
)

type UpgradeController struct {
	client         *unversioned.Client
	nodeStore      cache.Store
	nodeController *framework.Controller
	recorder       record.EventRecorder

	// The maximum duration before a pod evicted from a node can be forcefully terminated.
	maximumGracePeriod time.Duration

	podEvictor, terminationEvictor *node.RateLimitedTimedQueue
	evictorLock                    sync.Mutex // protects the evictors above
}

func NewUpgradeController(
	client *unversioned.Client,
	deletionEvictionLimiter util.RateLimiter,
	terminationEvictionLimiter util.RateLimiter,
) *UpgradeController {
	eventBroadcaster := record.NewBroadcaster()

	uc := &UpgradeController{
		client:             client,
		podEvictor:         node.NewRateLimitedTimedQueue(deletionEvictionLimiter),
		terminationEvictor: node.NewRateLimitedTimedQueue(terminationEvictionLimiter),
		recorder:           eventBroadcaster.NewRecorder(api.EventSource{Component: "upgradecontroller"}),
		maximumGracePeriod: maximumGracePeriod,
	}

	nodeLW := cache.NewListWatchFromClient(
		uc.client,
		"nodes",
		api.NamespaceAll,
		fields.Everything(),
	)

	uc.nodeStore, uc.nodeController = framework.NewInformer(
		nodeLW,
		&api.Node{},
		nodeResyncPeriod,
		framework.ResourceEventHandlerFuncs{},
	)

	return uc
}

// evictPods queues an eviction for the provided node name, and returns false if the node is already
// queued for eviction.
func (uc *UpgradeController) evictPods(nodeName string) bool {
	uc.evictorLock.Lock()
	defer uc.evictorLock.Unlock()
	return uc.podEvictor.Add(nodeName)
}

func (uc *UpgradeController) Run(stopCh <-chan struct{}) error {
	defer util.HandleCrash()
	go uc.nodeController.Run(stopCh)
	go util.Until(func() {
		if err := uc.cleanIncompatibleNodes(); err != nil {
			glog.Errorf("error cleaning incomaptible nodes: %v ", err)
		}
	}, nodeCleanPeriod, stopCh)

	// Managing eviction of nodes:
	// 1. when we delete pods off a node, if the node was not empty at the time we then
	//    queue a termination watcher
	//    a. If we hit an error, retry deletion
	// 2. The terminator loop ensures that pods are eventually cleaned and we never
	//    terminate a pod in a time period less than nc.maximumGracePeriod. AddedAt
	//    is the time from which we measure "has this pod been terminating too long",
	//    after which we will delete the pod with grace period 0 (force delete).
	//    a. If we hit errors, retry instantly
	//    b. If there are no pods left terminating, exit
	//    c. If there are pods still terminating, wait for their estimated completion
	//       before retrying
	go util.Until(func() {
		uc.evictorLock.Lock()
		defer uc.evictorLock.Unlock()
		uc.podEvictor.Try(func(value node.TimedValue) (bool, time.Duration) {
			remaining, err := uc.deletePods(value.Value)
			if err != nil {
				util.HandleError(fmt.Errorf("unable to evict node %q: %v", value.Value, err))
				return false, 0
			}

			if remaining {
				uc.terminationEvictor.Add(value.Value)
			}
			return true, 0
		})
	}, nodeEvictionPeriod, stopCh)

	go util.Until(func() {
		uc.evictorLock.Lock()
		defer uc.evictorLock.Unlock()
		uc.terminationEvictor.Try(func(value node.TimedValue) (bool, time.Duration) {
			completed, remaining, err := uc.terminatePods(value.Value, value.AddedAt)
			if err != nil {
				util.HandleError(fmt.Errorf("unable to terminate pods on node %q: %v", value.Value, err))
				return false, 0
			}

			if completed {
				glog.Infof("All pods terminated on %s", value.Value)
				uc.recordNodeEvent(value.Value, api.EventTypeNormal, "TerminatedAllPods", fmt.Sprintf("Terminated all Pods on Node %s.", value.Value))
				return true, 0
			}

			glog.V(2).Infof("Pods terminating since %s on %q, estimated completion %s", value.AddedAt, value.Value, remaining)
			// clamp very short intervals
			if remaining < nodeEvictionPeriod {
				remaining = nodeEvictionPeriod
			}
			return false, remaining
		})
	}, nodeEvictionPeriod, stopCh)

	return nil
}

func (uc *UpgradeController) cleanIncompatibleNodes() error {
	nodes := uc.nodeStore.List()

	for _, iface := range nodes {
		node := iface.(*api.Node)

		// skip compatible nodes
		if _, ok := node.Annotations[meta.IncompatibleExecutor]; !ok {
			continue
		}

		uc.evictPods(node.GetName())
	}

	return nil
}

// deletePods will delete all pods from master running on given node, and return true
// if any pods were deleted.
func (uc *UpgradeController) deletePods(nodeName string) (bool, error) {
	remaining := false
	selector := fields.OneTermEqualSelector(unversioned.PodHost, nodeName)
	options := api.ListOptions{FieldSelector: selector}
	pods, err := uc.client.Pods(api.NamespaceAll).List(options)
	if err != nil {
		return remaining, err
	}

	if len(pods.Items) > 0 {
		uc.recordNodeEvent(nodeName, api.EventTypeNormal, "DeletingAllPods", fmt.Sprintf("Deleting all Pods from Node %v.", nodeName))
	}

	for _, pod := range pods.Items {
		// Defensive check, also needed for tests.
		if pod.Spec.NodeName != nodeName {
			continue
		}
		// if the pod has already been deleted, ignore it
		if pod.DeletionGracePeriodSeconds != nil {
			continue
		}

		glog.V(2).Infof("Starting deletion of pod %v", pod.Name)
		uc.recorder.Eventf(&pod, api.EventTypeNormal, "NodeControllerEviction", "Marking for deletion Pod %s from Node %s", pod.Name, nodeName)
		if err := uc.client.Pods(pod.Namespace).Delete(pod.Name, nil); err != nil {
			return false, err
		}
		remaining = true
	}
	return remaining, nil
}

// terminatePods will ensure all pods on the given node that are in terminating state are eventually
// cleaned up. Returns true if the node has no pods in terminating state, a duration that indicates how
// long before we should check again (the next deadline for a pod to complete), or an error.
func (uc *UpgradeController) terminatePods(nodeName string, since time.Time) (bool, time.Duration, error) {
	// the time before we should try again
	nextAttempt := time.Duration(0)
	// have we deleted all pods
	complete := true

	selector := fields.OneTermEqualSelector(unversioned.PodHost, nodeName)
	options := api.ListOptions{FieldSelector: selector}
	pods, err := uc.client.Pods(api.NamespaceAll).List(options)
	if err != nil {
		return false, nextAttempt, err
	}

	now := time.Now()
	elapsed := now.Sub(since)
	for _, pod := range pods.Items {
		// Defensive check, also needed for tests.
		if pod.Spec.NodeName != nodeName {
			continue
		}
		// only clean terminated pods
		if pod.DeletionGracePeriodSeconds == nil {
			continue
		}

		// the user's requested grace period
		grace := time.Duration(*pod.DeletionGracePeriodSeconds) * time.Second
		if grace > uc.maximumGracePeriod {
			grace = uc.maximumGracePeriod
		}

		// the time remaining before the pod should have been deleted
		remaining := grace - elapsed
		if remaining < 0 {
			remaining = 0
			glog.V(2).Infof("Removing pod %v after %s grace period", pod.Name, grace)
			uc.recordNodeEvent(nodeName, api.EventTypeNormal, "TerminatingEvictedPod", fmt.Sprintf("Pod %s has exceeded the grace period for deletion after being evicted from Node %q and is being force killed", pod.Name, nodeName))
			if err := uc.client.Pods(pod.Namespace).Delete(pod.Name, api.NewDeleteOptions(0)); err != nil {
				glog.Errorf("Error completing deletion of pod %s: %v", pod.Name, err)
				complete = false
			}
		} else {
			glog.V(2).Infof("Pod %v still terminating, requested grace period %s, %s remaining", pod.Name, grace, remaining)
			complete = false
		}

		if nextAttempt < remaining {
			nextAttempt = remaining
		}
	}
	return complete, nextAttempt, nil
}

func (uc *UpgradeController) recordNodeEvent(nodeName, eventtype, reason, event string) {
	ref := &api.ObjectReference{
		Kind:      "Node",
		Name:      nodeName,
		UID:       types.UID(nodeName),
		Namespace: "",
	}
	glog.V(2).Infof("Recording %s event message for node %s", event, nodeName)
	uc.recorder.Eventf(ref, eventtype, reason, "Node %s event: %s", nodeName, event)
}
