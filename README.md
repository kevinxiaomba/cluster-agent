# AppDynamics ClusterAgent

**Thank you for your interest in the cluster-agent project.  We are actively seeking to integrate this functionality into an official AppDynamics beta offering, and as a result we are making this project private while we work on the integration.  If you are interested in participating in the beta program for our Kubernetes Cluster Agent, please contact your AppDynamics account manager.**

AppDynamics ClusterAgent is an application for monitoring workloads on Kubernetes clusters. It is implemented in Golang as a native Kubernetes component. The ClusterAgent is designed to work with AppDynamics controller and is associated with a specific AppDynamics tenant. 

The ClusterAgent has 2 purposes.
 
 * It collects metrics and state of Kubernetes resources and reports them to the AppDynamics controller.
 * It instruments AppDynamics application agents into workloads deployed to the Kubernetes cluster.


## Cluster monitoring
The ClusterAgent monitors state of Kubernetes resources and derives metrics to provide visibility into common application impacting issues. The metrics are displayed in the cluster overview dashboard and the snapshot data is stored in AppDynamics analytics engine for drill-downs and further analysis.

![Cluster Overview Dashboard](https://github.com/Appdynamics/cluster-agent/blob/master/docs/assets/cluster-dashboard.png)

 [Cluster monitoring overview](https://github.com/Appdynamics/cluster-agent/blob/master/docs/monitoring.md)



## Application instrumentation

The ClusterAgent can be configured to auto instrument Java and .Net Core workloads

[Application instrumentation overview](https://github.com/Appdynamics/cluster-agent/blob/master/docs/instrumentation.md)

## Prerequisites

* [Kubernetes Metrics server](https://github.com/kubernetes-incubator/metrics-server) enables collection of resource utilization metrics. If it is not already deployed to the cluster, run the following command:
`kubectl create -f metrics-server/`
* [An AppDynamics user account](https://github.com/Appdynamics/cluster-agent/blob/master/docs/rest-user-role.md) must be setup for the ClusterAgent to communicate to the AppDynamics controller via REST API.
* Access to AppDynamics Controller 4.5.5+
* At least 1 APM license (Golang) and 1 Analytics license. AppDynamics offers a [free trial](https://www.appdynamics.com/free-trial/)

## How to deploy

The ClusterAgent can be deployed and managed manually or with the [AppDynamics ClusterAgent Operator](https://github.com/Appdynamics/appdynamics-operator/blob/master/README.md). 

When deploying manually, follow these steps:

* Create namespace for AppDynamics components
  * Kubernetes
   `kubectl create namespace appdynamics`
  * OpenShift
   `oc new-project appdynamics --description="AppDynamics"`
* Update controller URL in the configMap (deploy/cluster-agent/cluster-agent-config.yaml). The controller URL must be in the following format:
` <protocol>://<controller-url>:<port> `

* Create Secret `cluster-agent-secret`. 
  * The "api-user" key with the AppDynamics user account information is required. It needs to be in the following format <username>@<account>:<password>, e.g ` user@customer1:123 `. 
  * The other 2 keys, "controller-key" and "event-key", are optional. If not specified, the ClusterAgent will attempt to obtain them automatically.

```
kubectl -n appdynamics create secret generic cluster-agent-secret \
--from-literal=api-user="username@customer1:password" \
--from-literal=controller-key="" \
--from-literal=event-key="" \
```

* Update the image reference in the ClusterAgent deployment spec, if necessary (deploy/cluster-agent/appd-cluster-agent.yaml). 

### Images

By default "docker.io/appdynamics/cluster-agent:latest" is used.

[AppDynamics images](https://access.redhat.com/containers/#/product/f5e13e601dc05eaa) are also available from [Red Hat Container Catalog](https://access.redhat.com/containers/). Here are the steps to enable pulling in OpenShift.

Create a secret in the ClusterAgent namespace. In this example, namespace **appdynamics** is used and appdynamics-operator account is linked to the secret.

```
$ oc -n appdynamics create secret docker-registry redhat-connect 
--docker-server=registry.connect.redhat.com 
--docker-username=REDHAT_CONNECT_USERNAME 
--docker-password=REDHAT_CONNECT_PASSWORD --docker-email=unused
$ oc -n appdynamics secrets link appdynamics-operator redhat-connect 
--for=pull 
```

To build your own image, use the provided ./build.sh script:

```
	./build.sh appdynamics/cluster-agent <version>
```

* Deploy the ClusterAgent
 `kubectl create -f deploy/cluster-agent/`



## Configuration properties

The ClusterAgent behavior is driven by configuration settings. Refer to the [list of configuration settings](https://github.com/Appdynamics/cluster-agent/blob/master/docs/configs.md) for details

## Support
Support is provided by the author. Please post your inquiries and bug reports in the [Issues](https://github.com/Appdynamics/cluster-agent/issues) area. To expedite issue resolution, please include the first 30 lines of logs with the ClusterAgent version and AppDynamics controller version, along with the log of the actual error. 

