# AppDynamics ClusterAgent

AppDynamics ClusterAgent is an application for monitoring workloads on Kubernetes clusters. It is implemented in Golang as a native Kubernetes component. The ClusterAgent is designed to work with AppDynamics controller and is associated with a specific AppDynamics tenant. 

The ClusterAgent has 2 purposes.
 
 * It collects metrics and state of Kubernetes resources and reports them to the AppDynamics controller.
 * It instruments AppDynamics application agents into workloads deployed to the Kuberenetes cluster.


## Cluster monitoring
The ClusterAgent monitors state of Kuberenetes resources and derives metrics to provide visibility into common application impacting issues. The metrics are displayed in the cluster overview dashboard and the snapshot data is stored in AppDynamics analytics engine for drill-downs and further analysis.

![Cluster Overview Dashboard](https://github.com/Appdynamics/cluster-agent/blob/master/docs/assets/cluster-dashboard.png)

 [Cluster monitoring overview](https://github.com/Appdynamics/cluster-agent/blob/master/docs/monitoring.md)



## Application instrumentation

The ClusterAgent can be configured to auto instrument Java and .Net Core workloads

[Application instrumentation overview](https://github.com/Appdynamics/cluster-agent/blob/master/docs/instrumentation.md)

## Prerequisites

* [Kuberenetes Metrics server](https://github.com/kubernetes-incubator/metrics-server) enables collection of resource utilization metrics. If it is not already deployed to the cluster, run the following command:
`kubectl create -f metrics-server/`
* [An AppDynamics user account](https://github.com/Appdynamics/cluster-agent/blob/master/docs/rest-user-role.md) must be setup for the ClusterAgent to communicate to the AppDynamics controller via REST API.
* Access to AppDynamics Controller 4.5.5+
* At least 1 APM license (Golang). AppDynamics offers a [free trial](https://www.appdynamics.com/free-trial/)

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


## Legal notice
The following complete example is provided as a preview of features that we are considering for a planned beta enhancement of our Kubernetes monitoring solution.

This Kubernetes monitoring solution has been developed using documented features of the AppDynamics Platform and we encourage customers to provide feedback about the functionality.  We will offer support for this preview offering on a best-efforts basis; requests for enhancements or additional features will be evaluated as input to our product roadmap.

AppDynamics reserves the right to change beta features at any time before making them generally available as well as never making them generally available. Any buying decisions should be made based on features and products that are currently generally available.  It is anticipated that this project will eventually be retired once equivalent functionality is fully incorporated into the AppDynamics Platform.

