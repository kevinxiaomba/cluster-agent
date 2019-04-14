# AppDynamics ClusterAgent

AppDynamics ClusterAgent is an application for monitoring workloads on Kubernetes clusters. It is implemented in Golang as a native Kubernetes component. The ClusterAgent is designed to work with AppDynamics controller and is associated with a specific AppDynamics tenant. 
The ClusterAgent has 2 purposes. 
 * It collects metrics and state of Kubernetes resources and reports them to an AppDynamics controller.
 * It instruments AppDynamics application agents into workloads deployed to the Kuberenetes cluster.


## Cluster monitoring
The ClusterAgent monitors state of Kuberenetes resources and derives metrics to provide visibility into the following common application impacting issues. The metrics are displayed in the cluster overview dashboard and the snapshot data is stored in AppDynamics analytics engine for drill-downs and further analysis.

![Cluster Overview Dashboard](https://github.com/Appdynamics/cluster-agent/blob/master/docs/assets/cluster-dashboard.png)

 [Cluster monitoring overview](https://github.com/Appdynamics/cluster-agent/blob/master/docs/monitoring.md)



## Application instrumentation

The Cluster agent can be configured to auto instrument Java and .Net workloads

[Application instrumentation overview](https://github.com/Appdynamics/cluster-agent/blob/master/docs/instrumentation.md)

## Quick start
The ClusterAgent can be deployed and managed manually or with a Kuberenetes Operator. 
The Kubernetes Operator is a recommended approach, as it hides a number of steps and compexities. For details see [Link]

* Create namespace for AppDynamics components
  * Kubernetes
   `kubectl create namespace appdynamics-infra`
  * OpenShift
   `oca new-project appdynamics-infra --description="AppDynamics Infrastructure"`
* Update controller URL in the configMap
* Create an AppDynamics account
* Create a Secret
`kubectl -n appdynamics-infra create secret generic cluster-agent-secret \
--from-literal=controller-key="" \
--from-literal=event-key="" \
--from-literal=api-user=""
`
"api-user" is required

* Deploy
 `kubectl create -f deploy/`

### Additional considerations

The ClusterAgent is designed to listen to updates to its configMap and use the new values of most of the settings without restart.


## Legal notice
The following complete example is provided as a preview of features that we are considering for a planned beta enhancement of our Kubernetes monitoring solution.

This Kubernetes monitoring solution has been developed using documented features of the AppDynamics Platform and we encourage customers to provide feedback about the functionality.  We will offer support for this preview offering on a best-efforts basis; requests for enhancements or additional features will be evaluated as input to our product roadmap.

AppDynamics reserves the right to change beta features at any time before making them generally available as well as never making them generally available. Any buying decisions should be made based on features and products that are currently generally available.  It is anticipated that this project will eventually be retired once equivalent functionality is fully incorporated into the AppDynamics Platform.

