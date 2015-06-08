## Getting started with Kubernetes on Mesosphere DCOS

<!-- TODO: Update, clean up. -->

[Mesosphere Data Center Operating System (DCOS)](TBA) allows enterprise grade deployments of Kubernetes in your local DCOS cluster or in the cloud with a single command.
With DCOS, your Kubernetes environment runs reliably alongside other popular next-generation services, on the same cluster without competing for resources.
In addition running Kubernetes on Mesos allows you to easily move Kubernetes workloads from one cloud provider to another to your own physical datacenter.

This tutorial will walk you through setting up Kubernetes on a DCOS cluster.
The walkthrough presented here is based on the v0.4.x series of the Kubernetes-Mesos project, which itself is based on Kubernetes v0.11.0.

### Prerequisites

* Understanding of [Apache Mesos](http://mesos.apache.org/)
* Running [DCOS cluster](link)
* Installed [DCOS command line interface](link)
* Indentified the hostname

### Installing Kubernetes on DCOS

1.  From the DCOS CLI, enter this command:

    ```
    $ dcos package install kubernetes
    ```

1.  Verify that Kubernetes is successfully installed:

    * From the DCOS CLI: `dcos package list`
    * From the DCOS web interface, go to the Services tab and confirm that Kubernetes running. You can click on the Kubernetes service to go the web interface.
     **Tip:** Kubernetes may take a few minutes to launch and display a healthy status.
    <img src="{% asset_path kubernetestask.png %}" alt="">
    * From the Mesos web interface at `<hostname>/mesos`, verify that the Kubernetes framework has registered and is starting tasks.


You can view usage metrics at `<hostname>/service/Kubernetes/metrics`. For more information, see <a href="https://github.com/GoogleCloudPlatform/kubernetes/blob/master/docs/getting-started-guides/mesos.md" target="_blank">Getting started with Kubernetes on Mesos</a>.

### <a name="usage"></a>Usage Examples

Prerequisites
 * Kubernetes service is up and running in DCOS
 * Docker is [installed](https://docs.docker.com/installation/)

#### Launch a Kubernetes Pod

In this example, a single Kubernetes pod is launched:

1.  Export the Kubernetes master URL:

    ```
    $ export KUBERNETES_MASTER=http://<hostname>/service/kubernetes/api
    ```

1.  Download the Kubernetes pod definition file:

    ```
    $ mkdir examples && curl -o examples/pod-nginx.json https://raw.githubusercontent.com/mesosphere/kubernetes-mesos/master/examples/pod-nginx.json
    ```

1.  Launch the Kubernetes pod:

    ```
    $ curl $KUBERNETES_MASTER/v1beta1/pods -XPOST -H'Content-type: json' -d@examples/pod-nginx.json
    ```

#### Using kubectl with Kubernetes on DCOS

In this example, a dockerized <a href="https://github.com/GoogleCloudPlatform/kubernetes/blob/release-0.14/docs/kubectl.md" target="_blank">kubectl</a> is used to interact with Kubernetes on DCOS:

1.  Export the Kubernetes master URL:

    ```
    $ export KUBERNETES_MASTER=http://<hostname>/service/kubernetes/api
    ```

1.  Create a helper alias that runs kubectl for DCOS in a Docker container:

    ```
    $ alias kubectl='docker run -e KUBERNETES_MASTER=$KUBERNETES_MASTER mesosphere/kubernetes:k8s-0.14.2-k8sm-0.5-dcos-20150603T1136520000 kc'
    ```

    **Tip:** Linux users might have to prefix the `docker run` command with `sudo`.

1.  Interact with Kubernetes:

    ```
    $ kubectl get pods
    ```

### Uninstalling Kubernetes

1.  Stop your pods and replication controllers and wait for any running pods to be cleaned up. Otherwise, after Kubernetes is removed Mesos will wait for the scheduler's failover-timeout interval to expire before killing the remaining pods. You can configure the scheduler's failover-timeout with the [dcos package install](/using/cli/packagesyntax/#usage) command.

1.  From the DCOS CLI, enter this command:

    ```
    $ dcos package uninstall kubernetes
    ```

