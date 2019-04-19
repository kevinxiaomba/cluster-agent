## ClusterAgent Configuration properties

The ClusterAgent is designed to listen to updates to its configMap and use the new values without restarts.

Changes to the following properties require restart:

* AgentNamespace 
* AppName 
* Account
* GlobalAccount
* AccessKey
* ControllerUrl
* EventKey
* RestAPICred

All configuration updates are transparently handled by [AppDynamics ClusterAgent Operator](https://github.com/Appdynamics/appdynamics-operator/blob/master/README.md).



### Required properties

***ControllerUrl***:				Full Url of the AppDynamics Controller in the following format:
								 `<protocol>://<controller-dns>:<port> `         



### Optional


#### General info


***Account***:          			AppDynamics account name, e.g. customer1

***GlobalAccount***:    			AppDynamics global account name

***EventServiceUrl***:  			Url to the AppDynamics event service. If not specified, is derived from the controller Url

***AppName*** :         			Name of the ClusterAgent application. The application will appear in AppDynamics UI under this name

***TierName***:         			Name of the ClusterAgent tier in AppDynamics

***NodeName***:         			Name of the ClusterAgent node in AppDynamics

***AgentServerPort***:  			Port number of the internal web server. Default is 8989

***SystemSSLCert***:    			Path to the system SSL certificate. Default is "/opt/appd/ssl/system.crt"

***AgentSSLCert***:            	Path to the agent SSL certificate. Default is "/opt/appd/ssl/agent.crt"

***ProxyUrl***:						Proxy server url in the following format <protocol>://<dns>:<port> 

***ProxyUser***:					Proxy user name

***ProxyPass***:					Proxy user password

***LogLevel***:                	Level of logging of the ClusterAgent application. Supported values: "info", "error", "debug". Default is 		
								*info*
								

#### Limits


***EventAPILimit***:           	Max number of analytics events when sent in a batch to the AppDynamics Events API

***MetricsSyncInterval***:     	Frequency of metrics updates in seconds. Default is 60

***SnapshotSyncInterval***:    	Frequency of snapshot updates in seconds. Default is 15

***LogLines***:                	Number of last lines to log when pod crashes. Default is 0 (logging disabled)

***PodEventNumber***:          	Number of last events to show on pod heat map. Default is 1

***OverconsumptionThreshold***:   Percent of resource utilization in a pod that triggers "Over-consume" flag. Default is 80



#### Monitoring


***NsToMonitor***:					List of namespaces to monitor

***NsToMonitorExclude***:			List of namespaces to exclude from monitoring

***NodesToMonitor***:				List of nodes to monitor

***NodesToMonitorExclude***:		List of nodes to exclude from monitoring



#### Analytics Schemas

***PodSchemaName***:           	Pods. Default is "kube_pod_snapshots"

***NodeSchemaName***:          	Nodes. Default is "kube_node_snapshots"

***EventSchemaName***:         	Events. Default is "kube_event_snapshots"

***ContainerSchemaName***:     	Containers. Default is "kube_container_snapshots"

***JobSchemaName***:           	Jobs. Default is "kube_jobs"

***LogSchemaName***:           	Pod logs. Default is "kube_logs"

***EpSchemaName***:            	Service endpoints. Default is "kube_endpoints"

***NsSchemaName***:            	Namespaces. Default is "kube_ns_snapshots"

***RqSchemaName***:            	Resource quotas. Default is "kube_rq_snapshots"

***RSSchemaName***:            	Replica sets. Default is "kube_rs_snapshots"

***DeploySchemaName***:        	Deployments. Default is "kube_deploy_snapshots"

***DaemonSchemaName***:        	Daemon sets. Default is "kube_daemon_snapshots"



#### Dashboarding


***DashboardTemplatePath***:		Path to the cluster overview template. Default is "/opt/appdynamics/templates/cluster-template.json"

***DashboardSuffix***:				Suffix in the name of the cluster overview dashboard 

***DashboardDelayMin***:			Number of seconds after the ClusterAgent start time when the first attempt to create th cluster overview 											dashboard is made. The default is 10

		
#### Agent Instrumentation


***InstrumentationMethod***:		Method of APM Instrumentation ("mountEnv", "mountAttach", "none"). Default is "none"

***DefaultInstrumentationTech***:	AppServer agent used for instrumentation by default ("java")

***NsToInstrument***:				List of namespaces included into instrumentation

***NsToInstrumentExclude***:		List of namespaces excluded from the instrumentation

***NSInstrumentRule***:			List of instrumentation rules. Each rule can be configured in the following format:

```
namespaces: # List of namespaces wher the rule applies. Required	
   - ns1  
 matchString: "client-api"  # Regex to match against deployment names and label values. Optional	
 appDAppLabel: "appName"  # Value of this label will become AppDynamics application name. Optional			
 appDTierLabel: "tierName"	  # Value of this label will become AppDynamics tier name. Optional			
 version: "appdynamics/java" # Agent image reference. Optional	
 tech: "java" # Type of agent to use. Optional	
 method: "mountenv" # Instrumentation method to use. Optional	
 biq: "sidecar"	 # Method of Analytics instrumentation
```

For details on instrumntation rules refer to [the Instrumentation overview](https://github.com/Appdynamics/cluster-agent/blob/master/docs/instrumentation.md)

***BiqService***:					How Analytics agent to be deployed ("sidecar", "url to remote service", "none").  Default is "none"

***InstrumentContainer***:			Containers to instrument by default if more than 1 are present in a pod ("first", "all", "specific name")

***InstrumentMatchString***:		List of strings to be matched against deployment names and label values for instrumentation

***AgentLabel***:					Label in deployment metadata to provide agent instrumentation information. Default is "appd-agent"

***AppDAppLabel***:					Label in deployment metadata to provide AppDynamics application name. Default is "appd-app"

***AppDTierLabel***:				Label in deployment metadata to provide AppDynamics tier name. Default is "appd-tier"

***AppDAnalyticsLabel***:			Label in deployment metadata to provide analytics instrumentation information. Default is "appd-biq"

***AgentMountName***:				Directory name for mounting  agent artifacts. Default is "appd-agent-repo"

***AgentMountPath***:				Directory path for mounting  agent artifacts. Default is "appd-agent-repo""/opt/appdynamics"

***AppLogMountName***:				Directory name for log analytics. Default is "appd-volume"

***AppLogMountPath***:				Directory path for log analytics. Default is "/opt/appdlogs"

***InitContainerDir***:			Directory in the generated init container where agent artifacts are copied. Default is "/opt/temp"

***AgentEnvVar***:					Environment variable for storing Java agent parameters. Default is "JAVA_OPTS"

***NodeNamePrefix***:				Node name prefix. Used when node reuse is set to true. If not set, the tier name is used

***AnalyticsAgentUrl***:			Url of the remote analytics agent. Default is "http://analytics-proxy:9090"

***AnalyticsAgentContainerName***:	Name of the generated analytics container. Default is "appd-analytics-agent"

***AppDInitContainerName**:		Init container name. Default is "appd-agent-attach"

***AnalyticsAgentImage***:			Reference to the analytics agent image. Default is "store/appdynamics/analytics-agent:latest"

***AppDJavaAttachImage***:			Reference to the java agent image. Default is "store/appdynamics/java-agent:latest"       
 
***AppDDotNetAttachImage***:		Reference to the .Net Core agent image. Default is "store/appdynamics/dotnt-agent:latest" 

***InitRequestMem***:				Memory request (MB) for the generated init container. Default is "50"

***InitRequestCpu***:				CPU request for the generated init container. Default is "0.1"

***BiqRequestMem***:				Memory request (MB) for the generated analytics sidecar container. Default is "600"

***BiqRequestCpu***:				CPU request for the generated analytics sidecar container. Default is "0.1"
		
		
		