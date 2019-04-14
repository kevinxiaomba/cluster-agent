## Cluster monitoring

The ClusterAgent monitors state of Kuberenetes resources and derives metrics to provide visibility into the following common application impacting issues. The metrics are displayed in the cluster overview dashboard and the snapshot data is stored in AppDynamics analytics engine for drill-downs and further analysis.

![Cluster Overview Dashboard](https://github.com/Appdynamics/cluster-agent/docs/assets/cluster-dashboard.png)

The collected data is grouped as follows:

* Application crashes
* Misconfiguration
* Missing dependencies
* Missing connectivity
* Resource starvation and overutilization
* Image issues
* Storage issues
* Resource utilization relative to capacity and limits

The metrics are pushed to the AppDynamics controller under the application name and the tier of the ClusterAgent. In addition, the raw snapshot data is sent to the Controller as Analytics events and can be viewed and further analyzed with ADQL.
A cluster-level dashboard with metrics is generated out-of-the-box. Deployment specific dashboards can be generated on demand, by changing the ClusterAgent configuration