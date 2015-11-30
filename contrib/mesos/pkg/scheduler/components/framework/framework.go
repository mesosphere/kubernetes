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

package framework

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	log "github.com/golang/glog"
	mesos "github.com/mesos/mesos-go/mesosproto"
	mutil "github.com/mesos/mesos-go/mesosutil"
	bindings "github.com/mesos/mesos-go/scheduler"
	"k8s.io/kubernetes/contrib/mesos/pkg/executor/messages"
	"k8s.io/kubernetes/contrib/mesos/pkg/node"
	"k8s.io/kubernetes/contrib/mesos/pkg/offers"
	offermetrics "k8s.io/kubernetes/contrib/mesos/pkg/offers/metrics"
	"k8s.io/kubernetes/contrib/mesos/pkg/proc"
	"k8s.io/kubernetes/contrib/mesos/pkg/runtime"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/components/tasksreconciler"
	schedcfg "k8s.io/kubernetes/contrib/mesos/pkg/scheduler/config"
	merrors "k8s.io/kubernetes/contrib/mesos/pkg/scheduler/errors"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/meta"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/metrics"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/podtask"
	"k8s.io/kubernetes/pkg/api"
	apierrors "k8s.io/kubernetes/pkg/api/errors"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/kubelet/container"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/sets"
)

var Unregistered = errors.New("failed to execute action, scheduler is disconnected")

type Framework interface {
	bindings.Scheduler

	Init(sched scheduler.Scheduler, electedMaster proc.Process, mux *http.ServeMux) error
	Registration() <-chan struct{}
	Offers() offers.Registry
	LaunchTask(t *podtask.T) error
	KillTask(id string) error
}

type framework struct {
	driver      bindings.SchedulerDriver // late initialization
	frameworkId *mesos.FrameworkID
	registered  bool
	mu          sync.RWMutex // proctects fields above

	// Config related, write-once
	sched             scheduler.Scheduler
	schedulerConfig   *schedcfg.Config
	client            *client.Client
	failoverTimeout   float64 // in seconds
	reconcileInterval int64
	nodeRegistrator   node.Registrator
	storeFrameworkId  func(id string)
	lookupNode        node.LookupFunc
	executorId        *mesos.ExecutorID

	// Mesos context
	registration   chan struct{} // signal chan that closes upon first successful registration
	onRegistration sync.Once
	offers         offers.Registry
	slaveHostNames *slaveRegistry

	// via deferred init
	tasksReconciler    taskreconciler.TasksReconciler
	mux                *http.ServeMux
	reconcileCooldown  time.Duration
	asRegisteredMaster proc.Doer
	terminate          <-chan struct{} // signal chan, closes when we should kill background tasks
}

type Config struct {
	SchedulerConfig   schedcfg.Config
	ExecutorId        *mesos.ExecutorID
	Client            *client.Client
	StoreFrameworkId  func(id string)
	FailoverTimeout   float64
	ReconcileInterval int64
	ReconcileCooldown time.Duration
	LookupNode        node.LookupFunc
}

// New creates a new Framework
func New(config Config) Framework {
	var f *framework
	f = &framework{
		schedulerConfig:   &config.SchedulerConfig,
		client:            config.Client,
		failoverTimeout:   config.FailoverTimeout,
		reconcileInterval: config.ReconcileInterval,
		nodeRegistrator:   node.NewRegistrator(config.Client, config.LookupNode),
		executorId:        config.ExecutorId,
		offers: offers.CreateRegistry(offers.RegistryConfig{
			Compat: func(o *mesos.Offer) bool {
				// the node must be registered and have up-to-date labels
				n := config.LookupNode(o.GetHostname())
				if n == nil || !node.IsUpToDate(n, node.SlaveAttributesToLabels(o.GetAttributes())) {
					return false
				}

				eids := len(o.GetExecutorIds())
				switch {
				case eids > 1:
					// at most one executor id expected. More than one means that
					// the given node is seriously in trouble.
					return false

				case eids == 1:
					// the executor id must match, otherwise the running executor
					// is incompatible with the current scheduler configuration.
					if eid := o.GetExecutorIds()[0]; eid.GetValue() != config.ExecutorId.GetValue() {
						return false
					}
				}

				return true
			},
			DeclineOffer: func(id string) <-chan error {
				errOnce := proc.NewErrorOnce(f.terminate)
				errOuter := f.asRegisteredMaster.Do(func() {
					var err error
					defer errOnce.Report(err)
					offerId := mutil.NewOfferID(id)
					filters := &mesos.Filters{}
					_, err = f.Driver().DeclineOffer(offerId, filters)
				})
				return errOnce.Send(errOuter).Err()
			},
			// remember expired offers so that we can tell if a previously scheduler offer relies on one
			LingerTTL:     config.SchedulerConfig.OfferLingerTTL.Duration,
			TTL:           config.SchedulerConfig.OfferTTL.Duration,
			ListenerDelay: config.SchedulerConfig.ListenerDelay.Duration,
		}),
		slaveHostNames:    newSlaveRegistry(),
		reconcileCooldown: config.ReconcileCooldown,
		registration:      make(chan struct{}),
		asRegisteredMaster: proc.DoerFunc(func(proc.Action) <-chan error {
			return proc.ErrorChanf("cannot execute action with unregistered scheduler")
		}),
		storeFrameworkId: config.StoreFrameworkId,
		lookupNode:       config.LookupNode,
	}
	return f
}

func (f *framework) Driver() bindings.SchedulerDriver {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.driver
}

func (f *framework) FrameworkID() *mesos.FrameworkID {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.frameworkId
}

func (f *framework) IsRegistered() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.registered
}

func (f *framework) Init(sched scheduler.Scheduler, electedMaster proc.Process, mux *http.ServeMux) error {
	log.V(1).Infoln("initializing kubernetes mesos scheduler")

	f.sched = sched
	f.mux = mux
	f.asRegisteredMaster = proc.DoerFunc(func(a proc.Action) <-chan error {
		if !f.IsRegistered() {
			return proc.ErrorChanf(Unregistered.Error())
		}
		return electedMaster.Do(a)
	})
	f.terminate = electedMaster.Done()
	f.offers.Init(f.terminate)
	f.nodeRegistrator.Run(f.terminate)
	return f.recoverTasks()
}

func (f *framework) asMaster() proc.Doer {
	return f.asRegisteredMaster
}

// An executorRef holds a reference to an executor and the slave it is running on
type executorRef struct {
	executorID *mesos.ExecutorID
	slaveID    *mesos.SlaveID
}

// executorRefs returns a slice of known references to running executors known to this framework
func (f *framework) executorRefs() []executorRef {
	slaves := f.slaveHostNames.SlaveIDs()
	refs := make([]executorRef, 0, len(slaves))

	for _, slaveID := range slaves {
		hostname := f.slaveHostNames.HostName(slaveID)
		if hostname == "" {
			log.Warningf("hostname lookup for slaveID %q failed", slaveID)
			continue
		}

		node := f.lookupNode(hostname)
		if node == nil {
			log.Warningf("node lookup for slaveID %q failed", slaveID)
			continue
		}

		eid, ok := node.Annotations[meta.ExecutorIdKey]
		if !ok {
			log.Warningf("unable to find %q annotation for node %v", meta.ExecutorIdKey, node)
			continue
		}

		refs = append(refs, executorRef{
			executorID: mutil.NewExecutorID(eid),
			slaveID:    mutil.NewSlaveID(slaveID),
		})
	}

	return refs
}

// kamikaze sends a kamikaze message to the underlying driver
func (f *framework) kamikaze(w io.Writer) error {
	refs := f.executorRefs()

	for _, ref := range refs {
		_, driverErr := f.Driver().SendFrameworkMessage(
			ref.executorID,
			ref.slaveID,
			messages.Kamikaze,
		)

		if driverErr != nil { // non-fatal
			msg := fmt.Sprintf(
				"error sending kamikaze message to executor %q on slave %q: %v",
				ref.executorID.GetValue(),
				ref.slaveID.GetValue(),
				driverErr,
			)
			log.Warning(msg)

			if _, err := fmt.Fprintln(w, msg); err != nil {
				return err
			}

			continue
		}

		_, err := io.WriteString(w, fmt.Sprintf(
			"kamikaze message sent to executor %q on slave %q\n",
			ref.executorID.GetValue(),
			ref.slaveID.GetValue(),
		))

		if err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "OK")
	return err
}

func (f *framework) installDebugHandlers(mux *http.ServeMux) {
	asMaster := func(a Action) Action {
		return ActionFunc(func(w io.Writer) error {
			if !f.IsRegistered() {
				return Unregistered
			}

			return a.Execute(w)
		})
	}

	guard := Guard(
		f.schedulerConfig.HttpHandlerTimeout.Duration,
		f.terminate,
	)

	handler := func(a Action) http.Handler {
		return Handler(Decorate(a, Logger, asMaster, guard))
	}

	action := func(f func()) Action {
		return ActionFunc(func(io.Writer) error {
			f()
			return nil
		})
	}

	mux.Handle(
		"/debug/actions/requestExplicit",
		handler(action(f.tasksReconciler.RequestExplicit)),
	)

	mux.Handle(
		"/debug/actions/requestImplicit",
		handler(action(f.tasksReconciler.RequestImplicit)),
	)

	mux.Handle(
		"/debug/actions/kamikaze",
		handler(ActionFunc(f.kamikaze)),
	)
}

func (f *framework) Registration() <-chan struct{} {
	return f.registration
}

// Registered is called when the scheduler registered with the master successfully.
func (f *framework) Registered(drv bindings.SchedulerDriver, fid *mesos.FrameworkID, mi *mesos.MasterInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	log.Infof("Scheduler registered with the master: %v with frameworkId: %v\n", mi, fid)

	f.driver = drv
	f.frameworkId = fid
	f.registered = true

	f.onRegistration.Do(func() { f.onInitialRegistration(drv) })
	f.tasksReconciler.RequestExplicit()
}

// Reregistered is called when the scheduler re-registered with the master successfully.
// This happends when the master fails over.
func (f *framework) Reregistered(drv bindings.SchedulerDriver, mi *mesos.MasterInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	log.Infof("Scheduler reregistered with the master: %v\n", mi)

	f.driver = drv
	f.registered = true

	f.onRegistration.Do(func() { f.onInitialRegistration(drv) })
	f.tasksReconciler.RequestExplicit()
}

// perform one-time initialization actions upon the first registration event received from Mesos.
func (f *framework) onInitialRegistration(driver bindings.SchedulerDriver) {
	defer close(f.registration)

	if f.failoverTimeout > 0 {
		refreshInterval := f.schedulerConfig.FrameworkIdRefreshInterval.Duration
		if f.failoverTimeout < f.schedulerConfig.FrameworkIdRefreshInterval.Duration.Seconds() {
			refreshInterval = time.Duration(math.Max(1, f.failoverTimeout/2)) * time.Second
		}
		go runtime.Until(func() {
			f.storeFrameworkId(f.FrameworkID().GetValue())
		}, refreshInterval, f.terminate)
	}

	r1 := f.makeTaskRegistryReconciler()
	r2 := f.makePodRegistryReconciler()

	f.tasksReconciler = taskreconciler.New(f.asRegisteredMaster, taskreconciler.MakeComposite(f.terminate, r1, r2),
		f.reconcileCooldown, f.schedulerConfig.ExplicitReconciliationAbortTimeout.Duration, f.terminate)
	go f.tasksReconciler.Run(driver, f.terminate)

	if f.reconcileInterval > 0 {
		ri := time.Duration(f.reconcileInterval) * time.Second
		time.AfterFunc(f.schedulerConfig.InitialImplicitReconciliationDelay.Duration, func() { runtime.Until(f.tasksReconciler.RequestImplicit, ri, f.terminate) })
		log.Infof("will perform implicit task reconciliation at interval: %v after %v", ri, f.schedulerConfig.InitialImplicitReconciliationDelay.Duration)
	}

	f.installDebugHandlers(f.mux)
}

// Disconnected is called when the scheduler loses connection to the master.
func (f *framework) Disconnected(driver bindings.SchedulerDriver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	log.Infof("Master disconnected!\n")

	f.registered = false

	// discard all cached offers to avoid unnecessary TASK_LOST updates
	f.offers.Invalidate("")
}

// ResourceOffers is called when the scheduler receives some offers from the master.
func (f *framework) ResourceOffers(driver bindings.SchedulerDriver, offers []*mesos.Offer) {
	log.V(2).Infof("Received offers %+v", offers)

	// Record the offers in the global offer map as well as each slave's offer map.
	f.offers.Add(offers)
	for _, offer := range offers {
		slaveId := offer.GetSlaveId().GetValue()
		f.slaveHostNames.Register(slaveId, offer.GetHostname())

		// create api object if not existing already
		if f.nodeRegistrator != nil {
			labels := node.SlaveAttributesToLabels(offer.GetAttributes())
			_, err := f.nodeRegistrator.Register(offer.GetHostname(), labels)
			if err != nil {
				log.Error(err)
			}
		}
	}
}

// OfferRescinded is called when the resources are recinded from the scheduler.
func (f *framework) OfferRescinded(driver bindings.SchedulerDriver, offerId *mesos.OfferID) {
	log.Infof("Offer rescinded %v\n", offerId)

	oid := offerId.GetValue()
	f.offers.Delete(oid, offermetrics.OfferRescinded)
}

// StatusUpdate is called when a status update message is sent to the scheduler.
func (f *framework) StatusUpdate(driver bindings.SchedulerDriver, taskStatus *mesos.TaskStatus) {
	source, reason := "none", "none"
	if taskStatus.Source != nil {
		source = (*taskStatus.Source).String()
	}
	if taskStatus.Reason != nil {
		reason = (*taskStatus.Reason).String()
	}
	taskState := taskStatus.GetState()
	metrics.StatusUpdates.WithLabelValues(source, reason, taskState.String()).Inc()

	message := "none"
	if taskStatus.Message != nil {
		message = *taskStatus.Message
	}

	log.Infof(
		"task status update %q from %q for task %q on slave %q executor %q for reason %q with message %q",
		taskState.String(),
		source,
		taskStatus.TaskId.GetValue(),
		taskStatus.SlaveId.GetValue(),
		taskStatus.ExecutorId.GetValue(),
		reason,
		message,
	)

	switch taskState {
	case mesos.TaskState_TASK_RUNNING, mesos.TaskState_TASK_FINISHED, mesos.TaskState_TASK_STARTING, mesos.TaskState_TASK_STAGING:
		if _, state := f.sched.Tasks().UpdateStatus(taskStatus); state == podtask.StateUnknown {
			if taskState != mesos.TaskState_TASK_FINISHED {
				//TODO(jdef) what if I receive this after a TASK_LOST or TASK_KILLED?
				//I don't want to reincarnate then..  TASK_LOST is a special case because
				//the master is stateless and there are scenarios where I may get TASK_LOST
				//followed by TASK_RUNNING.
				//TODO(jdef) consider running this asynchronously since there are API server
				//calls that may be made
				f.reconcileNonTerminalTask(driver, taskStatus)
			} // else, we don't really care about FINISHED tasks that aren't registered
			return
		}
		if hostName := f.slaveHostNames.HostName(taskStatus.GetSlaveId().GetValue()); hostName == "" {
			// a registered task has an update reported by a slave that we don't recognize.
			// this should never happen! So we don't reconcile it.
			log.Errorf("Ignore status %+v because the slave does not exist", taskStatus)
			return
		}
	case mesos.TaskState_TASK_FAILED, mesos.TaskState_TASK_ERROR:
		if task, _ := f.sched.Tasks().UpdateStatus(taskStatus); task != nil {
			if task.Has(podtask.Launched) && !task.Has(podtask.Bound) {
				go f.sched.Reconcile(task)
				return
			}
		} else {
			// unknown task failed, not much we can do about it
			return
		}
		// last-ditch effort to reconcile our records
		fallthrough
	case mesos.TaskState_TASK_LOST, mesos.TaskState_TASK_KILLED:
		f.reconcileTerminalTask(driver, taskStatus)
	default:
		log.Errorf(
			"unknown task status %q from %q for task %q on slave %q executor %q for reason %q with message %q",
			taskState.String(),
			source,
			taskStatus.TaskId.GetValue(),
			taskStatus.SlaveId.GetValue(),
			taskStatus.ExecutorId.GetValue(),
			reason,
			message,
		)
	}
}

func (f *framework) reconcileTerminalTask(driver bindings.SchedulerDriver, taskStatus *mesos.TaskStatus) {
	task, state := f.sched.Tasks().UpdateStatus(taskStatus)

	if (state == podtask.StateRunning || state == podtask.StatePending) &&
		((taskStatus.GetSource() == mesos.TaskStatus_SOURCE_MASTER && taskStatus.GetReason() == mesos.TaskStatus_REASON_RECONCILIATION) ||
			(taskStatus.GetSource() == mesos.TaskStatus_SOURCE_SLAVE && taskStatus.GetReason() == mesos.TaskStatus_REASON_EXECUTOR_TERMINATED) ||
			(taskStatus.GetSource() == mesos.TaskStatus_SOURCE_SLAVE && taskStatus.GetReason() == mesos.TaskStatus_REASON_EXECUTOR_UNREGISTERED) ||
			(taskStatus.GetSource() == mesos.TaskStatus_SOURCE_EXECUTOR && taskStatus.GetMessage() == messages.ContainersDisappeared)) {
		//--
		// pod-task has metadata that refers to:
		// (1) a task that Mesos no longer knows about, or else
		// (2) a pod that the Kubelet will never report as "failed"
		// (3) a pod that the kubeletExecutor reported as lost (likely due to docker daemon crash/restart)
		// For now, destroy the pod and hope that there's a replication controller backing it up.
		// TODO(jdef) for case #2 don't delete the pod, just update it's status to Failed
		pod := &task.Pod
		log.Warningf("deleting rogue pod %v/%v for lost task %v", pod.Namespace, pod.Name, task.ID)
		if err := f.client.Pods(pod.Namespace).Delete(pod.Name, api.NewDeleteOptions(0)); err != nil && !apierrors.IsNotFound(err) {
			log.Errorf("failed to delete pod %v/%v for terminal task %v: %v", pod.Namespace, pod.Name, task.ID, err)
		}
	} else if taskStatus.GetReason() == mesos.TaskStatus_REASON_EXECUTOR_TERMINATED || taskStatus.GetReason() == mesos.TaskStatus_REASON_EXECUTOR_UNREGISTERED {
		// attempt to prevent dangling pods in the pod and task registries
		log.V(1).Infof("request explicit reconciliation to clean up for task %v after executor reported (terminated/unregistered)", taskStatus.TaskId.GetValue())
		f.tasksReconciler.RequestExplicit()
	} else if taskStatus.GetState() == mesos.TaskState_TASK_LOST && state == podtask.StateRunning && taskStatus.ExecutorId != nil && taskStatus.SlaveId != nil {
		//TODO(jdef) this may not be meaningful once we have proper checkpointing and master detection
		//If we're reconciling and receive this then the executor may be
		//running a task that we need it to kill. It's possible that the framework
		//is unrecognized by the master at this point, so KillTask is not guaranteed
		//to do anything. The underlying driver transport may be able to send a
		//FrameworkMessage directly to the slave to terminate the task.
		log.V(2).Info("forwarding TASK_LOST message to executor %v on slave %v", taskStatus.ExecutorId, taskStatus.SlaveId)
		data := fmt.Sprintf("%s:%s", messages.TaskLost, task.ID) //TODO(jdef) use a real message type
		if _, err := driver.SendFrameworkMessage(taskStatus.ExecutorId, taskStatus.SlaveId, data); err != nil {
			log.Error(err.Error())
		}
	}
}

// reconcile an unknown (from the perspective of our registry) non-terminal task
func (f *framework) reconcileNonTerminalTask(driver bindings.SchedulerDriver, taskStatus *mesos.TaskStatus) {
	// attempt to recover task from pod info:
	// - task data may contain an api.PodStatusResult; if status.reason == REASON_RECONCILIATION then status.data == nil
	// - the Name can be parsed by container.ParseFullName() to yield a pod Name and Namespace
	// - pull the pod metadata down from the api server
	// - perform task recovery based on pod metadata
	taskId := taskStatus.TaskId.GetValue()
	if taskStatus.GetReason() == mesos.TaskStatus_REASON_RECONCILIATION && taskStatus.GetSource() == mesos.TaskStatus_SOURCE_MASTER {
		// there will be no data in the task status that we can use to determine the associated pod
		switch taskStatus.GetState() {
		case mesos.TaskState_TASK_STAGING:
			// there is still hope for this task, don't kill it just yet
			//TODO(jdef) there should probably be a limit for how long we tolerate tasks stuck in this state
			return
		default:
			// for TASK_{STARTING,RUNNING} we should have already attempted to recoverTasks() for.
			// if the scheduler failed over before the executor fired TASK_STARTING, then we should *not*
			// be processing this reconciliation update before we process the one from the executor.
			// point: we don't know what this task is (perhaps there was unrecoverable metadata in the pod),
			// so it gets killed.
			log.Errorf("killing non-terminal, unrecoverable task %v", taskId)
		}
	} else if podStatus, err := podtask.ParsePodStatusResult(taskStatus); err != nil {
		// possible rogue pod exists at this point because we can't identify it; should kill the task
		log.Errorf("possible rogue pod; illegal task status data for task %v, expected an api.PodStatusResult: %v", taskId, err)
	} else if name, namespace, err := container.ParsePodFullName(podStatus.Name); err != nil {
		// possible rogue pod exists at this point because we can't identify it; should kill the task
		log.Errorf("possible rogue pod; illegal api.PodStatusResult, unable to parse full pod name from: '%v' for task %v: %v",
			podStatus.Name, taskId, err)
	} else if pod, err := f.client.Pods(namespace).Get(name); err == nil {
		if t, ok, err := podtask.RecoverFrom(*pod); ok {
			log.Infof("recovered task %v from metadata in pod %v/%v", taskId, namespace, name)
			_, err := f.sched.Tasks().Register(t)
			if err != nil {
				// someone beat us to it?!
				log.Warningf("failed to register recovered task: %v", err)
				return
			} else {
				f.sched.Tasks().UpdateStatus(taskStatus)
			}
			return
		} else if err != nil {
			//should kill the pod and the task
			log.Errorf("killing pod, failed to recover task from pod %v/%v: %v", namespace, name, err)
			if err := f.client.Pods(namespace).Delete(name, nil); err != nil {
				log.Errorf("failed to delete pod %v/%v: %v", namespace, name, err)
			}
		} else {
			//this is pretty unexpected: we received a TASK_{STARTING,RUNNING} message, but the apiserver's pod
			//metadata is not appropriate for task reconstruction -- which should almost certainly never
			//be the case unless someone swapped out the pod on us (and kept the same namespace/name) while
			//we were failed over.

			//kill this task, allow the newly launched scheduler to schedule the new pod
			log.Warningf("unexpected pod metadata for task %v in apiserver, assuming new unscheduled pod spec: %+v", taskId, pod)
		}
	} else if apierrors.IsNotFound(err) {
		// pod lookup failed, should delete the task since the pod is no longer valid; may be redundant, that's ok
		log.Infof("killing task %v since pod %v/%v no longer exists", taskId, namespace, name)
	} else if apierrors.IsServerTimeout(err) {
		log.V(2).Infof("failed to reconcile task due to API server timeout: %v", err)
		return
	} else {
		log.Errorf("unexpected API server error, aborting reconcile for task %v: %v", taskId, err)
		return
	}
	if _, err := driver.KillTask(taskStatus.TaskId); err != nil {
		log.Errorf("failed to kill task %v: %v", taskId, err)
	}
}

// FrameworkMessage is called when the scheduler receives a message from the executor.
func (f *framework) FrameworkMessage(driver bindings.SchedulerDriver,
	executorId *mesos.ExecutorID, slaveId *mesos.SlaveID, message string) {
	log.Infof("Received messages from executor %v of slave %v, %v\n", executorId, slaveId, message)
}

// SlaveLost is called when some slave is lost.
func (f *framework) SlaveLost(driver bindings.SchedulerDriver, slaveId *mesos.SlaveID) {
	log.Infof("Slave %v is lost\n", slaveId)

	sid := slaveId.GetValue()
	f.offers.InvalidateForSlave(sid)

	// TODO(jdef): delete slave from our internal list? probably not since we may need to reconcile
	// tasks. it would be nice to somehow flag the slave as lost so that, perhaps, we can periodically
	// flush lost slaves older than X, and for which no tasks or pods reference.

	// unfinished tasks/pods will be dropped. use a replication controller if you want pods to
	// be restarted when slaves die.
}

// ExecutorLost is called when some executor is lost.
func (f *framework) ExecutorLost(driver bindings.SchedulerDriver, executorId *mesos.ExecutorID, slaveId *mesos.SlaveID, status int) {
	log.Infof("Executor %v of slave %v is lost, status: %v\n", executorId, slaveId, status)
}

// Error is called when there is an unrecoverable error in the scheduler or scheduler driver.
// The driver should have been aborted before this is invoked.
func (f *framework) Error(driver bindings.SchedulerDriver, message string) {
	log.Fatalf("fatal scheduler error: %v\n", message)
}

// filter func used for explicit task reconciliation, selects only non-terminal tasks which
// have been communicated to mesos (read: launched).
func explicitTaskFilter(t *podtask.T) bool {
	switch t.State {
	case podtask.StateRunning:
		return true
	case podtask.StatePending:
		return t.Has(podtask.Launched)
	default:
		return false
	}
}

// reconciler action factory, performs explicit task reconciliation for non-terminal
// tasks listed in the scheduler's internal taskRegistry.
func (k *framework) makeTaskRegistryReconciler() taskreconciler.Action {
	return taskreconciler.Action(func(drv bindings.SchedulerDriver, cancel <-chan struct{}) <-chan error {
		taskToSlave := make(map[string]string)
		for _, t := range k.sched.Tasks().List(explicitTaskFilter) {
			if t.Spec.SlaveID != "" {
				taskToSlave[t.ID] = t.Spec.SlaveID
			}
		}
		return proc.ErrorChan(k.explicitlyReconcileTasks(drv, taskToSlave, cancel))
	})
}

// reconciler action factory, performs explicit task reconciliation for non-terminal
// tasks identified by annotations in the Kubernetes pod registry.
func (f *framework) makePodRegistryReconciler() taskreconciler.Action {
	return taskreconciler.Action(func(drv bindings.SchedulerDriver, cancel <-chan struct{}) <-chan error {
		podList, err := f.client.Pods(api.NamespaceAll).List(labels.Everything(), fields.Everything())
		if err != nil {
			return proc.ErrorChanf("failed to reconcile pod registry: %v", err)
		}
		taskToSlave := make(map[string]string)
		for _, pod := range podList.Items {
			if len(pod.Annotations) == 0 {
				continue
			}
			taskId, found := pod.Annotations[meta.TaskIdKey]
			if !found {
				continue
			}
			slaveId, found := pod.Annotations[meta.SlaveIdKey]
			if !found {
				continue
			}
			taskToSlave[taskId] = slaveId
		}
		return proc.ErrorChan(f.explicitlyReconcileTasks(drv, taskToSlave, cancel))
	})
}

// execute an explicit task reconciliation, as per http://mesos.apache.org/documentation/latest/reconciliation/
func (f *framework) explicitlyReconcileTasks(driver bindings.SchedulerDriver, taskToSlave map[string]string, cancel <-chan struct{}) error {
	log.Info("explicit reconcile tasks")

	// tell mesos to send us the latest status updates for all the non-terminal tasks that we know about
	statusList := []*mesos.TaskStatus{}
	remaining := sets.StringKeySet(taskToSlave)
	for taskId, slaveId := range taskToSlave {
		if slaveId == "" {
			delete(taskToSlave, taskId)
			continue
		}
		statusList = append(statusList, &mesos.TaskStatus{
			TaskId:  mutil.NewTaskID(taskId),
			SlaveId: mutil.NewSlaveID(slaveId),
			State:   mesos.TaskState_TASK_RUNNING.Enum(), // req'd field, doesn't have to reflect reality
		})
	}

	select {
	case <-cancel:
		return merrors.ReconciliationCancelledErr
	default:
		if _, err := driver.ReconcileTasks(statusList); err != nil {
			return err
		}
	}

	start := time.Now()
	first := true
	for backoff := 1 * time.Second; first || remaining.Len() > 0; backoff = backoff * 2 {
		first = false
		// nothing to do here other than wait for status updates..
		if backoff > f.schedulerConfig.ExplicitReconciliationMaxBackoff.Duration {
			backoff = f.schedulerConfig.ExplicitReconciliationMaxBackoff.Duration
		}
		select {
		case <-cancel:
			return merrors.ReconciliationCancelledErr
		case <-time.After(backoff):
			for taskId := range remaining {
				if task, _ := f.sched.Tasks().Get(taskId); task != nil && explicitTaskFilter(task) && task.UpdatedTime.Before(start) {
					// keep this task in remaining list
					continue
				}
				remaining.Delete(taskId)
			}
		}
	}
	return nil
}

func (f *framework) recoverTasks() error {
	podList, err := f.client.Pods(api.NamespaceAll).List(labels.Everything(), fields.Everything())
	if err != nil {
		log.V(1).Infof("failed to recover pod registry, madness may ensue: %v", err)
		return err
	}
	recoverSlave := func(t *podtask.T) {

		slaveId := t.Spec.SlaveID
		f.slaveHostNames.Register(slaveId, t.Offer.Host())
	}
	for _, pod := range podList.Items {
		if _, isMirrorPod := pod.Annotations[kubetypes.ConfigMirrorAnnotationKey]; isMirrorPod {
			// mirrored pods are never reconciled because the scheduler isn't responsible for
			// scheduling them; they're started by the executor/kubelet upon instantiation and
			// reflected in the apiserver afterward. the scheduler has no knowledge of them.
			continue
		}
		if t, ok, err := podtask.RecoverFrom(pod); err != nil {
			log.Errorf("failed to recover task from pod, will attempt to delete '%v/%v': %v", pod.Namespace, pod.Name, err)
			err := f.client.Pods(pod.Namespace).Delete(pod.Name, nil)
			//TODO(jdef) check for temporary or not-found errors
			if err != nil {
				log.Errorf("failed to delete pod '%v/%v': %v", pod.Namespace, pod.Name, err)
			}
		} else if ok {
			f.sched.Tasks().Register(t)
			recoverSlave(t)
			log.Infof("recovered task %v from pod %v/%v", t.ID, pod.Namespace, pod.Name)
		}
	}
	return nil
}

func (f *framework) KillTask(id string) error {
	killTaskId := mutil.NewTaskID(id)
	_, err := f.Driver().KillTask(killTaskId)
	return err
}

func (f *framework) LaunchTask(t *podtask.T) error {
	taskInfo, err := t.BuildTaskInfo()
	if err != nil {
		return err
	}

	// assume caller is holding scheduler lock
	taskList := []*mesos.TaskInfo{taskInfo}
	offerIds := []*mesos.OfferID{t.Offer.Details().Id}
	filters := &mesos.Filters{}

	_, err = f.Driver().LaunchTasks(offerIds, taskList, filters)
	return err
}

func (f *framework) Offers() offers.Registry {
	return f.offers
}
