## AppDynamics User Security Requirements

The ClusterAgent requires a special user account to communicate to the AppDynamics controller.
To create the account, log into the AppDynamics controller and navigate to Administration - Users - Create
 
### Required permissions
The following permissions need to be assigned to the usr account directly or to a Role associated with the user

* Account
    * Administration
	* View and configure licenses
* Applications
	* Create, View, Edit Applications
* Analytics
	 	* General - Manage logs, fields, APIs and metrics
	* Searches - Create Searches
	* Events - View event data from all applications (Transactions, Custom Analytics Events)
* Dashboards
	* Create, View, Edit, Delete
	
### Event API key

The ClusterAgent sends snapshots of the state of Kubernetes resources to the AppDynamics controller at a configurable interval. For this operation an Event API key is required. If the user account is configured with the required permissions, the ClusterAgent will generate the key automatically and will store it in the ClusterAgent secret.


#### Generate event API key manually
 
Follow these steps to create the event API key manually:

In the AppDynamics controller UI, navigate to
Analytics - Configuration - API keys - Add
Name the key and set the following permissions:
* Custom Analytics Events permissions
	* Can manage schema
	* Can query all Custom Analytics Events
	* Can publish all Custom Analytics Events

