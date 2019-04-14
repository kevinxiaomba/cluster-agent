## Application instrumentation

The ClusterAgent uses a declarative approach to agent instrumentation, which is consistent with Kubernetes design principles. The agent instrumentation is initiated by changing the deployment spec of the apps that need to be monitored. The ClusterAgent adds an init container with the desired agent image to the deployment. The initcontainer copies the agent binaries to a shared volume on the pod and make them available to the main application container. The required agent parameters are passed to the main application container as environment variables. 
In addition to this method, some Java workloads can be also instrumented using Java dynamic attach.
Once an application is instrumented, the ClusterAgent associates the pod with the AppDynamics application/tier/node ids. For Java workloads, the association is implemented down to the node id. For other technologies, the association is at the app/tier level. The ids of the corresponding AppDynamics entities are reflected in the pod's annotations.
By default, the instrumentation is disabled. The instrumentation is controlled by several configuration settings.
InstrumentationMethod "none", "mountEnv", "mountAttach"
NSToInstrument
NSToInstrumentExclude
NSInstrumentRule

To enable instrumentation, the InstrumentationMethod must be either mountEnv or mountAttach and NSToInstrument must have at least 1 namespace.

The instrumentation can be declared at a deployment level or via ClusterAgent configuration. The ClusterAgent makes the instrumentation decision in this order:
Is the instrumentation enabled? InstrumentationMethod is not "none" and the deployment namespace is not excluded.
Is there a deployment metadata?
Is there a rule that matches the deployment?
Is there a namespace-wide rule that matches the deployment	

### Deployment metadata

`appd-app: marvel
 appd-agent: dotnet`


### ClusterAgent configuration
* Global defaults
* Specific rule
	* Tech, multiple namespaces, first container
	* Tech, multiple namespaces, specific container
	* Tech, multiple namespaces, multiple containers
	* Tech, multiple namespaces, specific container, method override
* Namespace-wide rule