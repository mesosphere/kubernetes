Example Usage:

```
export KUBERNETES_PROVIDER=mesos/docker
./cluster/kube-up.sh
(source ./cluster/${KUBERNETES_PROVIDER}/util.sh && test-e2e)
./cluster/kube-down.sh
```

[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/cluster/mesos/docker/README.md?pixel)]()
