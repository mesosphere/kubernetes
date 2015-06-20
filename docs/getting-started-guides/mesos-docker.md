## Getting Started With Kubernetes on Mesos on Docker

Since all of the required components can run in docker, this cluster requires the least possible resources while still
allowing most end-to-end tests to pass.

### Prerequisites

- [Docker CLI](https://docs.docker.com/)
- [Docker Engine](https://docs.docker.com/)

    On Mac, use [Boot2Docker](http://boot2docker.io/) or [Docker Machine](https://docs.docker.com/machine/install-machine/)
    to run Docker Engine in a linux VM.
- [Optional] [Virtual Box](https://www.virtualbox.org/wiki/Downloads)

    Required by Boot2Docker and Docker Machine
- [Optional] [etcd](https://github.com/coreos/etcd)

    Only used locally by integration tests, when building without `KUBE_RELEASE_RUN_TESTS=N`.

Note: On Mac, it's possible to install all the above via [Homebrew](http://brew.sh/).

```
brew install caskroom/cask/brew-cask
brew cask install virtualbox
brew install docker
brew install boot2docker
boot2docker init
boot2docker up
brew install etcd
```

### Walkthrough

1. Build Kubernetes-Mesos binaries (cross-compiled)

    ```
    KUBERNETES_CONTRIB=mesos KUBE_RELEASE_RUN_TESTS=N ./build/release.sh
    ```

    Environment Variables:
    - `KUBERNETES_CONTRIB=mesos` enables building of the mesos-specific binaries.
    - `KUBE_RELEASE_RUN_TESTS=N` disables the unit and integration tests that are by default run after the binaries are built.

1. Build docker images

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

1. Run End-To-End Tests

    ```
    ./cluster/test-e2e.sh
    ```

    Notable parameters:
    - Increase the logging verbosity: `-v=0`
    - Run only a subset of the tests (regex matching): `-ginkgo.focus=<pattern>`

1. Destroy cluster

    ```
    ./cluster/kube-down.sh
    ```


### Helpful scripts

- Kill all docker containers

    ```
    docker ps -q -a | xargs docker rm -f
    ```

- Clean up docker volumes

    ```
    docker run -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/docker:/var/lib/docker --rm martin/docker-cleanup-volumes
    ```

[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/docs/getting-started-guides/mesos-docker.md?pixel)]()
