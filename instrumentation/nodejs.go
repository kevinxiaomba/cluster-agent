package instrumentation

//npm install appdynamics@<x.y.z>

//determine the entry point
//run npm
//copy shim into the app dir
//reference the main app in the shim
//start the shim

//shim
//require("appdynamics").profile({
//  controllerHostName: '<controller host name>',
//  controllerPort: <controller port number>,
//  controllerSslEnabled: false,  // Set to true if controllerPort is SSL
//  accountName: '<AppDynamics_account_name>',
//  accountAccessKey: '<AppDynamics_account_key>', //required
//  applicationName: 'your_app_name',
//  tierName: 'choose_a_tier_name',
//  reuseNodePrefix: tier_name_node
//  reuseNode: true
// });
