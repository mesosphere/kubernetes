Example Usage:

Build all the docker images:

```
cd <kubernetes-mesos>
IMAGE_TAG=latest ./docker/build/build.sh
./docker/test/build.sh
./docker/mesos-slave/build.sh
SOURCE_DIR=<kubernetes> ./docker/km/build.sh
```

Build the binaries, up the cluster, run the tests, down the cluster
```
cd <kubernetes>
KUBERNETES_CONTRIB=mesos KUBE_RELEASE_RUN_TESTS=N ./build/release.sh
export KUBERNETES_PROVIDER=mesos/docker
./cluster/kube-up.sh
(cd cluster/mesos/docker; ./deployAddons.sh)
./cluster/test-e2e.sh -alsologtostderr=true -v=0 # optionally focus: -ginkgo.focus=Pods
./cluster/kube-down.sh
```

[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/cluster/mesos/docker/README.md?pixel)]()
