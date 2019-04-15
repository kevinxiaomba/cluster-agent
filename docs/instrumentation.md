## Application instrumentation

### Overview

The ClusterAgent uses a declarative approach to agent instrumentation, which is consistent with Kubernetes design principles. 
The agent instrumentation is initiated by changing the deployment spec of the apps that need to be monitored. The ClusterAgent adds an init container with the desired agent image to the deployment. The init container copies the agent binaries to a shared volume on the pod and make them available to the main application container. The required agent parameters are passed to the main application container as environment variables. 

In addition to this method, some Java workloads can be also instrumented using Java dynamic attach.

Once an application is instrumented, the ClusterAgent associates the pod with the AppDynamics application/tier/node ids. For Java workloads, the association is implemented down to the node id. For other technologies, the association is at the app/tier level. The ids of the corresponding AppDynamics entities are reflected in the pod's annotations.

By default, the instrumentation is disabled. The instrumentation is controlled by several configuration settings.
* InstrumentationMethod "none", "mountEnv", "mountAttach" (only applies to Java). When set to "mountEnv", the init container will be created along wth the necessary environment variables. When set to "mountAttach", the init container will be created with the Java agent artifacts, the artifacts will be mounted to the application container and live attach will be performed.

* NSToInstrument - list of namspaces with instrumentation enabled
* NSToInstrumentExclude - list of namspaces excluded from the instrumentation
* NSInstrumentRule - list of specific instrumentation rules. The rules can be applied to one or more namspaces. The can be granular to target specific deployments. Each instrumentation rule has the following structure: 

```	  
	  namespaces:
	    - prod						# List of namespaces wher the rule applies. Required	
	  matchString: "client-api"	# Regex to match against deployment names and label values. Optional	
	  appDAppLabel: "appName"		# Value of this label will become AppDynamics application name. Optional			
	  appDTierLabel: "tierName"	# Value of this label will become AppDynamics tier name. Optional			
	  version: "appdynamics/java"	# Agent image reference. Optional	
	  tech: "java"					# Type of agent to use. Optional	
	  method: "mountenv"			# Instrumentation method to use. Optional	
      biq: "sidecar"				# Method of Analytics instrumentation
	  containerName: "first"		# Regex supported match string to identify the container in the pod. Other options: "first", "all". Default is "first"
```

### Enabling instrumentation
To enable instrumentation, the InstrumentationMethod must be either mountEnv or mountAttach and NSToInstrument must have at least 1 namespace.

The instrumentation can be declared at a deployment level or via ClusterAgent configuration. 
The ClusterAgent makes the instrumentation decision in this order:
Is the instrumentation enabled? InstrumentationMethod is not "none" and the deployment namespace is not excluded.
Is there a deployment metadata?
Is there a rule that matches the deployment?
Is there a namespace-wide rule that matches the deployment	

### Deployment metadata
When requesting instrumentation in the deployment metadata, use the following labels

```
appd-app: "myapp"			# AppDynamics application name. Required
appd-tier: "mytier"  		# deployment name is used by default
appd-agent: "dotnet" 		# optional to override the system-wide default set in "DefaultInstrumentationTech"
```


### ClusterAgent configuration use cases
Below are several use cases that show varios configuration settings for instrumentation.

#### Instrument using global defaults
All applications deployed to specific namespaces are Java. All apps leverage $JAVA_OPTS environment variable

```
InstrumentationMethod: "mountEnv"
NSToInstrument:
	- ns1
	- ns2
```


#### Instrument using namespace-wide rules
One namespace is for Java workloads. Another is for .Net Core microservices

```instrumentRule:
	- namespaces:
	    - java-namspace
	  appDAppLabel: "appName"
	  appDTierLabel: "tierName"	# if not set, deployment name is used
	  tech: "java"					# if not set, "DefaultInstrumentationTech" is used
	- namespaces:
	    - netcore-namspace
	  appDAppLabel: "appName"
	  appDTierLabel: "tierName"	# if not set, deployment name is used
	  tech: "dotnet"
```


#### Instrument using more granular rules
* Instrumnent Java into the first app container in specific namespaces, when deployment name matches "*api":
```
 instrumentRule:
	- matchString: "*api"          # if not set, all deployments in the namespace are considered
	  namespaces:
	    - ns1
	  appDAppLabel: "appName"
	  appDTierLabel: "tierName"	# if not set, deployment name is used
	  tech: "java"					# if not set, "DefaultInstrumentationTech" is used
	  method: "mountEnv"			# if not set, "InstrumentationMethod" is used
	  containerName: "first"		# if not set, "InstrumentContainer" is used
```

* Instrumnent Java into the "api-container" app container in specific namespaces, when deployment name matches "*api". Also instrument Analytics
```
 instrumentRule:
	- matchString: "*api"
	  namespaces:
	    - ns1
	  appDAppLabel: "appName"
	  appDTierLabel: "tierName"	# if not set, deployment name is used
	  tech: "java"
	  method: "mountEnv"			# if not set, "DefaultInstrumentationTech" is used
      biq: "sidecar"				# if not set, "BiqService" is used
	  containerName: "api-container"
```

* Instrumnent .Net Core into the all app container in specific namespaces, when deployment name matches "*api". Also instrument Analytics
```
 instrumentRule:
	- matchString: "*api"
	  namespaces:
	    - ns1
	  appDAppLabel: "appName"		 
	  appDTierLabel: "tierName"	# if not set, deployment name is used
	  tech: "dotnet"
	  method: "mountEnv"			# if not set, "DefaultInstrumentationTech" is used
      biq: "sidecar"				# if not set, "BiqService" is used
	  containerName: "all"
```