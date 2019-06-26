## Application instrumentation

### Overview

The ClusterAgent uses a declarative approach to agent instrumentation, which is consistent with Kubernetes design principles. 
The agent instrumentation is initiated by changing the deployment spec of the apps that need to be monitored. The ClusterAgent adds an init container with the desired agent image to the deployment. The init container copies the agent binaries to a shared volume on the pod and make them available to the main application container. The required agent parameters are passed to the main application container as environment variables. 

In addition to this method, some Java workloads can be also instrumented using Java dynamic attach.

Once an application is instrumented, the ClusterAgent associates the pod with the AppDynamics application/tier/node ids. For Java workloads, the association is implemented down to the node id. For other technologies, the association is at the app/tier level. The ids of the corresponding AppDynamics entities are reflected in the pod annotations.

By default, the instrumentation is disabled. The instrumentation is controlled by several configuration settings.
* InstrumentationMethod "none", "mountEnv", "mountAttach" (only applies to Java). When set to "mountEnv", the init container will be created along with the necessary environment variables. When set to "mountAttach", the init container will be created with the Java agent artifacts, the artifacts will be mounted to the application container and live attach will be performed.

* NSToInstrument - list of namspaces with instrumentation enabled
* NSToInstrumentExclude - list of namspaces excluded from the instrumentation
* NSInstrumentRule - list of specific instrumentation rules. The rules can be applied to one or more namspaces. The can be granular to target specific deployments. Each instrumentation rule has the following structure: 

```	  
namespaces:
- ns1						# List of namespaces wher the rule applies. Required
matchString: 
- "client-api"	# Regex to match against deployment names and label values. Optional
appDAppLabel: "appName"		# Value of this label will become AppDynamics application name. Optional			
appDTierLabel: "tierName"	# Value of this label will become AppDynamics tier name. Optional			
version: "appdynamics/java"	# Agent image reference. Optional
tech: "java"					# Type of agent to use. Optional
method: "mountEnv"			# Instrumentation method to use. Optional
biq: "sidecar"				# Method of Analytics instrumentation
containerName: "first"		# Regex supported match string to identify the container in the pod. Other options: "first", "all". Default is "first"
```

### Enabling instrumentation
To enable instrumentation, the InstrumentationMethod must be set to mountEnv or mountAttach and NSToInstrument must have at least 1 namespace or a matching instrumentation rule is defined.

The instrumentation can be declared at a deployment level or via ClusterAgent configuration.

 
The ClusterAgent makes the instrumentation decision in this order:

* Is the instrumentation enabled? InstrumentationMethod is not "none" and the deployment namespace is not excluded.
* Are there known labels in the deployment metadata?
* Is there a rule that matches the deployment name or labels?
* Is there a namespace-wide rule that matches the deployment	name or labels

### Deployment metadata
When requesting instrumentation in the deployment metadata, use the following labels

```
appd-app: "myapp"			# AppDynamics application name. Required
appd-tier: "mytier"  		# Optional. The deployment name is used by default
appd-agent: "dotnet" 		# Optional. Alternatively, the system-wide default "DefaultInstrumentationTech" is used
```


### ClusterAgent configuration use cases
Below are several use cases with examples of instrumentation settings.

#### Use case 1
All Java deployments in namespaces ns1 will be instrumented. These apps leverage `$JAVA_OPTS` environment variable. All deployments will be grouped under application `appd-application01` in AppDynamics.

```
instrumentationMethod: "mountEnv"
appNameLiteral: appd-application01
nsToInstrument:
- ns1
```

#### Use case 2 
All Java applications with the name or metadata labels matching "client-api"
and deployed to namespace ns 1 will be instrumented. All apps leverage $JAVA_OPTS environment variable. All deployments will be grouped under application `appd-application01` in AppDynamics.

```
instrumentationMethod: "mountEnv"
appNameLiteral: appd-application01
instrumentMatchString:
- client-api
nsToInstrument:
- ns1
```


#### Use case 3
Use instrumentation rule to instrument Java apps with the name matching "java-app" in namespace ns1. Use the value of pod label "name" for the AppDynamics application name. Add analytics agent as a sidecar.

```instrumentRule:
  - appDAppLabel: name
    namespaces:
    - ns1
    matchString: 
    - java-app
    biQ: sidecar
```


#### Use case 4
Use instrumentation rules to instrument Java apps with the name matching "java-app"  and .Net Core apps with the name matching "dotnet-app" in namespace ns1.  Use the value of pod label "name" for the AppDynamics application name. Add analytics agent as a sidecar to the Java application.


```
instrumentRule:
  - appDAppLabel: name
    biQ: sidecar
    matchString:
    - java-app
    namespaces:
    - dev
  - appDAppLabel: name
    matchString:
    - dotnet-app
    namespaces:
    - dev
    tech: dotnet
```

#### Use case 5
Use instrumentation rule to instrument Java apps with the name matching "java-app" in namespace ns1. Use the value of pod label "name" for the AppDynamics application name. 
The Java agent will be logging to a custom location, /dev/myapp/logs. All directories with AppDynamics agent artifacts will be accessible for read/write operations by user 2000 / group 2000.

```instrumentRule:
  - appDAppLabel: name
    namespaces:
    - ns1
    matchString: 
    - java-app
    agentLogOverride: /dev/myapp/logs
    agentUserOverride: 2000:2000
```

#### Use case 6
All Java deployments in namespaces ns1 will be instrumented. These apps leverage a custom `$APM_OPTIONS` environment variable. All deployments will be grouped under application `appd-application01` in AppDynamics.

```
instrumentationMethod: "mountEnv"
appNameLiteral: appd-application01
agentEnvVar: APM_OPTIONS
nsToInstrument:
- ns1
```

#### Use case 7. Network visibility
All Java deployments in namespaces ns1 will be instrumented with Network visibility. These apps leverage a custom `$APM_OPTIONS` environment variable. All deployments will be grouped under application `appd-application01` in AppDynamics.

```
instrumentationMethod: "mountEnv"
appNameLiteral: appd-application01
agentEnvVar: APM_OPTIONS
netVizPort: 3892
nsToInstrument:
- ns1
```

#### Use case 8
To remove AppDynamics application agents from the application deployments, change one of the following instrumentation settings in the `clusteragent` spec:

* instrumentationMethod (set to "none")
* instrumentMatchString
* nsToInstrument
* agentEnvVar
* appNameLiteral
* appDAppLabel
* biqService

If, following the configuration change, the instrumented deployments no longer match the filter or their properties differ from the original instrumentation request (e.g. the Application Name in AppDynamics has changed) the specs of the affectet deployments will be reverted to the original state (before the AppDynamics instrumentation).
Note that following the de-instrumentation, the old application with its data will remain in the AppDynamics controller until explicitely removed by an authorized user.