## Getting Started

### Prerequisites

- [Docker CLI](https://docs.docker.com/installation/)
- [Docker Engine]
    On Mac, use [boot2docker](http://boot2docker.io/) or [Docker Machine](https://docs.docker.com/machine/install-machine/)
    to run Docker Engine in a linux VM.
- [Optional] [Virtual Box](https://www.virtualbox.org/wiki/Downloads)
    Required by boot2docker and Docker Machine
- [Optional] etcd
    (Only used locally by integration tests, when building without `KUBE_RELEASE_RUN_TESTS=N`.)

### Walkthrough

1. Build Kubernetes-Mesos binaries (cross-compiled)

    ```
    KUBERNETES_CONTRIB=mesos KUBE_RELEASE_RUN_TESTS=N ./build/release.sh
    ```

    Environment Variables:
    - `KUBERNETES_CONTRIB=mesos` enables building of the mesos-specific binaries.
    - `KUBE_RELEASE_RUN_TESTS=N` disables the unit and integration tests that are by default run after the binaries are built.

1. Build docker images

    Test image is used for running e2e tests.

    ```
    ./cluster/mesos/docker/test/build.sh
    ```

    Mesos-Slave image includes iptables & docker-in-docker.

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
