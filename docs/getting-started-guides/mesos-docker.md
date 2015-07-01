## Getting Started With Kubernetes on Mesos on Docker

The mesos/docker provider uses docker-compose to launch Kubernetes as a Mesos framework, running in docker with its
dependencies (etcd & mesos).

### Cluster Goals

- kubernetes development
- pod/service development
- demoing
- fast deployment
- minimal hardware requirements
- minimal configuration
- entry point for exploration
- simplified networking
- fast end-to-end tests
- local deployment

Non-Goals:
- high availability
- fault tolerance
- remote deployment
- production usage
- monitoring
- long running
- state persistence across restarts

### Cluster Topology

The cluster consists of several docker containers linked together by docker-managed hostnames:

| Component                     | Hostname                    | Description                                                                             |
|-------------------------------|-----------------------------|-----------------------------------------------------------------------------------------|
| docker-grand-ambassador       |                             | Proxy to allow circular hostname linking in docker                                      |
| etcd                          | etcd                        | Key/Value store used by Mesos                                                           |
| Mesos Master                  | mesosmaster1                | REST endpoint for interacting with Mesos                                                |
| Mesos Slave (x2)              | mesosslave1<br/>mesosslave2 | Mesos agents that offer resources and run framework executors (e.g. Kubernetes Kublets) |
| Kubernetes API Server         | apiserver                   | REST endpoint for interacting with Kubernetes                                           |
| Kubernetes Controller Manager | controller                  |                                                                                         |
| Kubernetes Scheduler          | scheduler                   | Schedules container deployment by accepting Mesos offers                                |

### Prerequisites

Required:
- [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git) - version control system
- [Docker CLI](https://docs.docker.com/) - container management command line client
- [Docker Engine](https://docs.docker.com/) - container management daemon
  - On Mac, use [Boot2Docker](http://boot2docker.io/) or [Docker Machine](https://docs.docker.com/machine/install-machine/)
- [Docker Compose](https://docs.docker.com/compose/install/) - multi-container application orchestration
- [jq](http://stedolan.github.io/jq/) - command line JSON parser

Optional:
- [Virtual Box](https://www.virtualbox.org/wiki/Downloads) - x86 hardware virtualizer
  - Required by Boot2Docker and Docker Machine
- [etcd](https://github.com/coreos/etcd) - key/value store<br/>
  - Required to run the integration tests locally (e.g. building without `KUBE_RELEASE_RUN_TESTS=N`)

#### Install on Mac (Homebrew)

It's possible to install all of the above via [Homebrew](http://brew.sh/) on a Mac.

Some steps print instructions for configuring or launching. Make sure each is properly set up before continuing to the next step.

```
brew install git
brew install jq
brew install caskroom/cask/brew-cask
brew cask install virtualbox
brew install docker
brew install boot2docker
boot2docker init
boot2docker up
brew install docker-compose
brew install etcd
```

#### Install on Linux

Most of the above are available via apt and yum, but depending on your distribution, you may have to install via other
means to get the latest versions.

It is recommended to use Ubuntu, simply because it best supports AUFS, used by docker to mount volumes. Alternate file
systems may not fully support docker-in-docker.


#### Boot2Docker Config (Mac)

If on a mac using boot2docker, the following steps will make the docker IPs (in the virtualbox VM) reachable from the
host machine (mac).

1. Set the VM's host-only network to "promiscuous mode":

    ```
    boot2docker stop
    VBoxManage modifyvm boot2docker-vm --nicpromisc2 allow-all
    boot2docker start
    ```

    This allows the VM to accept packets that were sent to a different IP.

    Since the host-only network routes traffic between VMs and the host, other VMs will also be able to access the docker
    IPs, if they have the following route.

1. Route traffic to docker through the boot2docker IP:

    ```
    sudo route -n add -net 172.17.0.0 $(boot2docker ip)
    ```

    Since the boot2docker IP can change when the VM is restarted, this route may need to be updated over time.
    To delete the route later: `sudo route delete 172.17.0.0`


### Walkthrough

1. Checkout the Kubernetes source

    ```
    git clone https://github.com/GoogleCloudPlatform/kubernetes
    cd kubernetes
    ```

    By default, that will get you the bleeding edge of master branch.
    You may want a [release branch](https://github.com/GoogleCloudPlatform/kubernetes/releases) instead,
    if you have trouble with master.

1. Build Kubernetes-Mesos binaries (cross-compiled)

    ```
    KUBERNETES_CONTRIB=mesos KUBE_RELEASE_RUN_TESTS=N ./build/release.sh
    ```

    Environment Variables:
    - `KUBERNETES_CONTRIB=mesos` enables building of the mesos-specific binaries.
    - `KUBE_RELEASE_RUN_TESTS=N` disables the unit and integration tests that are by default run after the binaries are built.

1. [Optional] Build docker images

    The following docker images are built as part of `./cluster/kube-up.sh`, but it may make sense to build them manually
    the first time because it may take a while. In the future some of these may be hosted publicly, but you will always
    need to at least rebuild the Kubernetes-Mesos image when using locally built binaries.

    Test image includes all the dependencies required for running e2e tests.

    ```
    ./cluster/mesos/docker/test/build.sh
    ```

    Mesos-Slave image extends the Mesosphere mesos-slave image to include iptables & docker-in-docker.

    ```
    ./cluster/mesos/docker/mesos-slave/build.sh
    ```

    Kubernetes-Mesos image includes the compiled linux binaries.

    ```
    ./cluster/mesos/docker/km/build.sh
    ```

1. Configure Mesos-Docker Provider

    ```
    export KUBERNETES_PROVIDER=mesos/docker
    ```

    ***Resources***

    It's optionally possible to modify the amount of resources the mesos-slaves will offer for Kubernetes to use.
    To do so, find the `MESOS_RESOURCES` environment variables in `./cluster/mesos/docker/docker-compose.yml` and modify
    them to your liking. Because mesos-slave resource auto-detection overlaps work when multiple slaves are on the same
    node, these have to be explicitly configured.

    ***Important***: The default mesos resource may or may not actually be available on your Docker Engine machine.
    You may have to increase you VM disk, memory, or cpu allocation in VirtualBox,
    [Docker Machine](https://docs.docker.com/machine/#oracle-virtualbox), or
    [Boot2Docker](https://ryanfb.github.io/etc/2015/01/28/increasing_boot2docker_allocations_on_os_x.html).

1. Create cluster

    ```
    ./cluster/kube-up.sh
    ```

    After deploying th cluster, `~/.kube/config` will be created or updated to configure kubectl to target the new cluster.

1. Explore Examples

   To learn more about Pods, Volumes, Labels, Services, and Replication Controllers, start with the
   [Kubernetes Walkthrough](/examples/walkthrough/README.md).

   To skip to a more advanced example, see the [Guestbook Example](/examples/guestbook/README.md)

1. Destroy cluster

    ```
    ./cluster/kube-down.sh
    ```


### End To End Testing

Warning: e2e tests can take a long time to run. You may not want to run them immediately if you're just getting started.

While your cluster is up, you can run the end-to-end tests:

```
./cluster/test-e2e.sh
```

Notable parameters:
- Increase the logging verbosity: `-v=2`
- Run only a subset of the tests (regex matching): `-ginkgo.focus=<pattern>`

To build, deploy, test, and destroy, all in one command (plus unit & integration tests):

```
make test_e2e
```


### Kubernetes CLI

When compiling from source, it's simplest to use the `./cluster/kubectl.sh` script, which detects your platform &
architecture and proxies commands to the appropriate `kubectl` binary.

ex: `./cluster/kubectl.sh get pods`


### Helpful scripts

- Kill all docker containers

    ```
    docker ps -q -a | xargs docker rm -f
    ```

- Clean up unused docker volumes

    ```
    docker run -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/docker:/var/lib/docker --rm martin/docker-cleanup-volumes
    ```

[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/docs/getting-started-guides/mesos-docker.md?pixel)]()
