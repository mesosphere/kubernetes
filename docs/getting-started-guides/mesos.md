## Getting started with Kubernetes on Mesos

<!-- TODO: Update, clean up. -->

Mesos allows dynamic sharing of cluster resources between Kubernetes and other first-class Mesos frameworks such as [Hadoop][1], [Spark][2], and [Chronos][3].
Mesos ensures applications running on your cluster are isolated and that resources are allocated fairly.

Running Kubernetes on Mesos allows you to easily move Kubernetes workloads from one cloud provider to another to your own physical datacenter.

This tutorial will walk you through setting up Kubernetes on a Mesos cluster on [Google Cloud Plaform][4].
It provides a step by step walk through of adding Kubernetes to a Mesos cluster and running the classic GuestBook demo application.

### Prerequisites

* Mesos cluster on [Google Compute Engine][5]
* [VPN connection to the cluster][6]

### Deploy Kubernetes-Mesos

Log into the master node over SSH, replacing the placeholder below with the correct IP address.

```bash
ssh jclouds@${ip_address_of_master_node}
```

Build Kubernetes-Mesos.

```bash
$ git clone https://github.com/mesosphere/kubernetes-mesos k8sm
$ mkdir -p bin && sudo docker run --rm -v $(pwd)/bin:/target \
  -v $(pwd)/k8sm:/snapshot -e GIT_BRANCH=release-0.4 \
  mesosphere/kubernetes-mesos:build
```

Set some environment variables.
The internal IP address of the master is visible via the cluster details page on the Mesosphere launchpad, or may be obtained via `hostname -i`.

```bash
$ export servicehost=$(hostname -i)
$ export mesos_master=${servicehost}:5050
$ export KUBERNETES_MASTER=http://${servicehost}:8888
```

Start etcd and verify that it is running:

```bash
$ sudo docker run -d --hostname $(hostname -f) --name etcd -p 4001:4001 -p 7001:7001 coreos/etcd
```

```bash
$ sudo docker ps
CONTAINER ID   IMAGE                COMMAND   CREATED   STATUS   PORTS                NAMES
fd7bac9e2301   coreos/etcd:latest   "/etcd"   5s ago    Up 3s    2379/tcp, 2380/...   etcd
```

Start the kubernetes-mesos API server, controller manager, scheduler, and proxy:

```bash
$ ./bin/km apiserver \
  --address=${servicehost} \
  --mesos_master=${mesos_master} \
  --etcd_servers=http://${servicehost}:4001 \
  --portal_net=10.10.10.0/24 \
  --port=8888 \
  --cloud_provider=mesos \
  --v=1 >apiserver.log 2>&1 &

$ ./bin/km controller-manager \
  --master=$servicehost:8888 \
  --mesos_master=${mesos_master} \
  --v=1 >controller.log 2>&1 &

$ ./bin/km scheduler \
  --address=${servicehost} \
  --mesos_master=${mesos_master} \
  --etcd_servers=http://${servicehost}:4001 \
  --mesos_user=root \
  --api_servers=$servicehost:8888 \
  --v=2 >scheduler.log 2>&1 &

$ sudo ./bin/km proxy \
  --bind_address=${servicehost} \
  --etcd_servers=http://${servicehost}:4001 \
  --logtostderr=true >proxy.log 2>&1 &
```

Disown your background jobs so that they'll stay running if you log out.

```bash
$ disown -a
```

Interact with the kubernetes-mesos framework via `kubectl`:

```bash
$ bin/kubectl get pods
POD        IP        CONTAINER(S)        IMAGE(S)        HOST        LABELS        STATUS
```

```bash
$ bin/kubectl get services       # your service IPs will likely differ
NAME            LABELS                                    SELECTOR            IP             PORT
kubernetes      component=apiserver,provider=kubernetes   <none>              10.10.10.2     443
kubernetes-ro   component=apiserver,provider=kubernetes   <none>              10.10.10.1     80
```

## Spin up a pod

Write a JSON pod description to a local file:

```bash
$ cat <<EOPOD >nginx.json
{ "kind": "Pod",
"apiVersion": "v1beta1",
"id": "nginx-id-01",
"desiredState": {
  "manifest": {
    "version": "v1beta1",
    "containers": [{
      "name": "nginx-01",
      "image": "dockerfile/nginx",
      "ports": [{
        "containerPort": 80,
        "hostPort": 31000
      }],
      "livenessProbe": {
        "enabled": true,
        "type": "http",
        "initialDelaySeconds": 30,
        "httpGet": {
          "path": "/index.html",
          "port": "8081"
        }
      }
    }]
  }
},
"labels": {
  "name": "foo"
} }
EOPOD
```

Send the pod description to Kubernetes using the `kubectl` CLI:

```bash
$ bin/kubectl create -f nginx.json
nginx-id-01
```

Wait a minute or two while `dockerd` downloads the image layers from the internet.
We can use the `kubectl` interface to monitor the status of our pod:

```bash
$ bin/kubectl get pods
POD          IP           CONTAINER(S)  IMAGE(S)          HOST                       LABELS                STATUS
nginx-id-01  172.17.5.27  nginx-01      dockerfile/nginx  10.72.72.178/10.72.72.178  cluster=gce,name=foo  Running
```

Verify that the pod task is running in the Mesos web console.

Now we can interact with the pod running on the Mesos cluster:

```bash
$ curl http://${slave_ip}:31000/
... (HTML, Welcome to nginx on Debian!)
```

## Run the Example Guestbook App

Following the instructions from the kubernetes-mesos [examples/guestbook][7]:

```bash
$ export ex=k8sm/examples/guestbook
$ bin/kubectl create -f $ex/redis-master.json
$ bin/kubectl create -f $ex/redis-master-service.json
$ bin/kubectl create -f $ex/redis-slave-controller.json
$ bin/kubectl create -f $ex/redis-slave-service.json
$ bin/kubectl create -f $ex/frontend-controller.json

$ cat <<EOS >/tmp/frontend-service
{
  "id": "frontend",
  "kind": "Service",
  "apiVersion": "v1beta1",
  "port": 9998,
  "selector": {
    "name": "frontend"
  },
  "publicIPs": [
    "${servicehost}"
  ]
}
EOS
$ bin/kubectl create -f /tmp/frontend-service
```

Watch your pods transition from `Pending` to `Running`:

```bash
$ watch 'bin/kubectl get pods'
```

Review your Mesos cluster's tasks:

```bash
$ mesos ps
   TIME   STATE    RSS     CPU    %MEM  COMMAND USER ID
 0:00:03    R    67.62 MB  0.5   105.66   none  root c7315c47-7a61-11e4-8b8c-42010a863922
 0:00:03    R    68.20 MB  0.75  106.57   none  root c731613c-7a61-11e4-8b8c-42010a863922
 0:00:01    R    67.79 MB  0.25  105.93   none  root c7310460-7a61-11e4-8b8c-42010a863922
 0:00:03    R    67.62 MB  0.5   105.66   none  root bda74154-7a61-11e4-8b8c-42010a863922
 0:00:03    R    68.20 MB  0.75  106.57   none  root bda6e97f-7a61-11e4-8b8c-42010a863922
 0:00:03    R    68.20 MB  0.75  106.57   none  root b0e206f1-7a61-11e4-8b8c-42010a863922
```

Determine the internal IP address of the frontend [service portal][8]:

```bash
$ bin/kubectl get services
NAME            LABELS                                    SELECTOR            IP             PORT
kubernetes      component=apiserver,provider=kubernetes   <none>              10.10.10.2     443
kubernetes-ro   component=apiserver,provider=kubernetes   <none>              10.10.10.1     80
redismaster     <none>                                    name=redis-master   10.10.10.49    10000
redisslave      name=redisslave                           name=redisslave     10.10.10.109   10001
frontend        <none>                                    name=frontend       10.10.10.149   9998
```

Interact with the frontend application via curl:

```bash
$ curl http://${frontend_service_ip_address}:9998/index.php?cmd=get\&key=messages
```

Or via the Redis CLI:

```bash
$ sudo apt-get install redis-tools
$ redis-cli -h ${redis_master_service_ip_address} -p 10000
10.233.254.108:10000> dump messages
"\x00\x06,world\x06\x00\xc9\x82\x8eHj\xe5\xd1\x12"
```

Tail the logs of the kubelet-executor:

```bash
$ mesos tail -f c7315c47-7a61-11e4-8b8c-42010a863922 stderr
I1202 20:34:38.749340 03297 log.go:151] GET /podInfo?podID=c7312b26-7a61-11e4-8b8c-42010a863922&podNamespace=default: (3.71265ms) 200
I1202 20:34:38.772047 03297 log.go:151] GET /podInfo?podID=bda7385a-7a61-11e4-8b8c-42010a863922&podNamespace=default: (3.785459ms) 200
...
```

Or interact with the frontend application via your browser, in 2 steps:

First, open the firewall on the master machine.

```bash
$ # determine the internal port for the frontend service portal
$ sudo iptables-save|grep -e frontend  # -- port 36336 in this case
-A KUBE-PORTALS-CONTAINER -d 10.10.10.149/32 -p tcp -m comment --comment frontend -m tcp --dport 9998 -j DNAT --to-destination 10.22.183.23:36336
-A KUBE-PORTALS-CONTAINER -d 10.22.183.23/32 -p tcp -m comment --comment frontend -m tcp --dport 9998 -j DNAT --to-destination 10.22.183.23:36336
-A KUBE-PORTALS-HOST -d 10.10.10.149/32 -p tcp -m comment --comment frontend -m tcp --dport 9998 -j DNAT --to-destination 10.22.183.23:36336
-A KUBE-PORTALS-HOST -d 10.22.183.23/32 -p tcp -m comment --comment frontend -m tcp --dport 9998 -j DNAT --to-destination 10.22.183.23:36336

$ # open up access to the internal port for the frontend service portal
$ sudo iptables -A INPUT -i eth0 -p tcp -m state --state NEW,ESTABLISHED -m tcp \
  --dport ${internal_frontend_service_port} -j ACCEPT
```

Next, add a firewall rule in Google Cloud Platform Console / Networking:

![Google Cloud Platform firewall configuration][9]

Now, you can visit the guestbook in your browser!

![Kubernetes Guestbook app running on Mesos][10]

[1]: http://mesosphere.com/docs/tutorials/run-hadoop-on-mesos-using-installer
[2]: http://mesosphere.com/docs/tutorials/run-spark-on-mesos
[3]: http://mesosphere.com/docs/tutorials/run-chronos-on-mesos
[4]: http://cloud.google.com
[5]: https://google.mesosphere.com
[6]: http://mesosphere.com/docs/getting-started/cloud/google/mesosphere/#vpn-setup
[7]: https://github.com/mesosphere/kubernetes-mesos/tree/v0.4.0/examples/guestbook
[8]: https://github.com/GoogleCloudPlatform/kubernetes/blob/v0.11.0/docs/services.md#ips-and-portals
[9]: mesos/k8s-firewall.png
[10]: mesos/k8s-guestbook.png
