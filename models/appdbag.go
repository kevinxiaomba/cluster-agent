package models

import (
	"fmt"
	"reflect"
	"strings"
)

type AppDBag struct {
	AgentNamespace              string
	AppName                     string
	TierName                    string
	NodeName                    string
	AppID                       int
	TierID                      int
	NodeID                      int
	Account                     string
	GlobalAccount               string
	AccessKey                   string
	ControllerUrl               string
	ControllerPort              uint16
	RestAPIUrl                  string
	SSLEnabled                  bool
	SystemSSLCert               string
	AgentSSLCert                string
	EventKey                    string
	EventServiceUrl             string
	RestAPICred                 string
	EventAPILimit               int
	PodSchemaName               string
	NodeSchemaName              string
	DeploySchemaName            string
	RSSchemaName                string
	DaemonSchemaName            string
	EventSchemaName             string
	ContainerSchemaName         string
	EpSchemaName                string
	NsSchemaName                string
	RqSchemaName                string
	JobSchemaName               string
	LogSchemaName               string
	DashboardTemplatePath       string
	DashboardSuffix             string
	DashboardDelayMin           int
	AgentEnvVar                 string
	AgentLabel                  string
	AppNameLiteral              string
	AppDAppLabel                string
	AppDTierLabel               string
	AppDAnalyticsLabel          string
	AgentLogOverride            string
	AgentUserOverride           string
	AgentMountName              string
	AgentMountPath              string
	AppLogMountName             string
	AppLogMountPath             string
	JDKMountName                string
	JDKMountPath                string
	NodeNamePrefix              string
	AnalyticsAgentUrl           string
	AnalyticsAgentImage         string
	AnalyticsAgentContainerName string
	AppDInitContainerName       string
	AppDJavaAttachImage         string
	AppDDotNetAttachImage       string
	AppDNodeJSAttachImage       string
	ProxyUrl                    string
	ProxyHost                   string
	ProxyPort                   string
	ProxyUser                   string
	ProxyPass                   string
	InitContainerDir            string
	MetricsSyncInterval         int // Frequency of metrics pushes to the controller, sec
	SnapshotSyncInterval        int // Frequency of snapshot pushes to events api, sec
	AgentServerPort             int
	NetVizPort                  int
	NsToMonitor                 []string
	NsToMonitorExclude          []string
	DeploysToDashboard          []string
	NodesToMonitor              []string
	NodesToMonitorExclude       []string
	NsToInstrument              []string
	NsToInstrumentExclude       []string
	NSInstrumentRule            []AgentRequest
	InstrumentationMethod       InstrumentationMethod
	DefaultInstrumentationTech  TechnologyName
	BiqService                  string
	InstrumentContainer         string //all, first, name
	InstrumentMatchString       []string
	InitRequestMem              string
	InitRequestCpu              string
	BiqRequestMem               string
	BiqRequestCpu               string
	LogLines                    int //0 - no logging
	PodEventNumber              int
	RemoteBiqProtocol           string
	RemoteBiqHost               string
	RemoteBiqPort               int
	SchemaUpdateCache           []string
	SchemaSkipCache             []string
	LogLevel                    string
	OverconsumptionThreshold    int //percent
	ControllerVer1              int
	ControllerVer2              int
	ControllerVer3              int
	ControllerVer4              int
	InstrumentationUpdated      bool
}

type AgentStatus struct {
	Version                    string
	MetricsSyncInterval        int
	SnapshotSyncInterval       int
	LogLevel                   string
	LogLines                   int
	NsToMonitor                []string
	NsToMonitorExclude         []string
	NodesToMonitor             []string
	NodesToMonitorExclude      []string
	NsToInstrument             []string
	NsToInstrumentExclude      []string
	NSInstrumentRule           []AgentRequest
	InstrumentationMethod      InstrumentationMethod
	DefaultInstrumentationTech TechnologyName
	InstrumentMatchString      []string
	BiqService                 string
	AnalyticsAgentImage        string
	AppDJavaAttachImage        string
	AppDDotNetAttachImage      string
}

func IsUpdatable(fieldName string) bool {
	arr := []string{"AgentNamespace", "AppName", "TierName", "NodeName", "AppID", "TierID", "NodeID", "Account", "GlobalAccount", "AccessKey", "ControllerUrl",
		"ControllerPort", "RestAPIUrl", "SSLEnabled", "SystemSSLCert", "AgentSSLCert", "EventKey", "EventServiceUrl", "RestAPICred"}
	for _, s := range arr {
		if s == fieldName {
			return false
		}
	}
	return true
}

func UpdateField(fieldName string, current *reflect.Value, updated *reflect.Value) {
	arr := []string{"PodSchemaName",
		"NodeSchemaName",
		"DeploySchemaName",
		"RSSchemaName",
		"DaemonSchemaName",
		"EventSchemaName",
		"ContainerSchemaName",
		"EpSchemaName",
		"NsSchemaName",
		"RqSchemaName",
		"JobSchemaName",
		"LogSchemaName"}

	found := false
	for _, s := range arr {
		if s == fieldName {
			found = true
			break
		}
	}

	if found && strings.ToLower(updated.String()) == "skip" {
		newVal := fmt.Sprintf("%s^%s", current.String(), updated.String())
		current.SetString(newVal)
	} else {
		current.Set(*updated)
	}

}

/// current controller version is -1 = older, 1 - newer or equal
func (bag *AppDBag) CompareControllerVersions(ver1 int, ver2 int, ver3 int, ver4 int) int {
	if bag.ControllerVer1 > 0 && bag.ControllerVer2 > 0 && bag.ControllerVer3 > 0 && bag.ControllerVer4 > 0 {
		if bag.ControllerVer1 > ver1 {
			return 1
		} else if bag.ControllerVer1 == ver1 && bag.ControllerVer2 > ver2 {
			return 1
		} else if bag.ControllerVer2 == ver2 && bag.ControllerVer3 > ver3 {
			return 1
		} else if bag.ControllerVer3 == ver3 && bag.ControllerVer4 >= ver4 {
			return 1
		} else {
			return -1
		}
	}
	return 1
}

func (bag *AppDBag) PrintControllerVersion() string {
	return fmt.Sprintf("%d.%d.%d.%d", bag.ControllerVer1, bag.ControllerVer2, bag.ControllerVer3, bag.ControllerVer4)
}

func (self *AppDBag) EnsureDefaults() {
	bag := GetDefaultProperties()
	if self.PodSchemaName == "" {
		self.PodSchemaName = bag.PodSchemaName
	}
	if self.NodeSchemaName == "" {
		self.NodeSchemaName = bag.NodeSchemaName
	}
	if self.EventSchemaName == "" {
		self.EventSchemaName = bag.EventSchemaName
	}
	if self.ContainerSchemaName == "" {
		self.ContainerSchemaName = bag.ContainerSchemaName
	}
	if self.JobSchemaName == "" {
		self.JobSchemaName = bag.JobSchemaName
	}
	if self.LogSchemaName == "" {
		self.LogSchemaName = bag.LogSchemaName
	}
	if self.EpSchemaName == "" {
		self.EpSchemaName = bag.EpSchemaName
	}
	if self.NsSchemaName == "" {
		self.NsSchemaName = bag.NsSchemaName
	}
	if self.RqSchemaName == "" {
		self.RqSchemaName = bag.RqSchemaName
	}
	if self.DeploySchemaName == "" {
		self.DeploySchemaName = bag.DeploySchemaName
	}
	if self.RSSchemaName == "" {
		self.RSSchemaName = bag.RSSchemaName
	}
	if self.DaemonSchemaName == "" {
		self.DaemonSchemaName = bag.DaemonSchemaName
	}
}

func GetDefaultProperties() *AppDBag {
	bag := AppDBag{
		AppName:                     "K8s-Cluster-Agent",
		TierName:                    "ClusterAgent",
		NodeName:                    "Node1",
		AgentServerPort:             8989,
		SystemSSLCert:               "/opt/appdynamics/ssl/appdsaascert.pem",
		AgentSSLCert:                "",
		EventAPILimit:               100,
		MetricsSyncInterval:         60,
		SnapshotSyncInterval:        30,
		PodSchemaName:               "kube_pod_snapshots",
		NodeSchemaName:              "kube_node_snapshots",
		EventSchemaName:             "kube_event_snapshots",
		ContainerSchemaName:         "kube_container_snapshots",
		JobSchemaName:               "kube_jobs",
		LogSchemaName:               "kube_logs",
		EpSchemaName:                "kube_endpoints",
		NsSchemaName:                "kube_ns_snapshots",
		RqSchemaName:                "kube_rq_snapshots",
		DeploySchemaName:            "kube_deploy_snapshots",
		RSSchemaName:                "kube_rs_snapshots",
		DaemonSchemaName:            "kube_daemon_snapshots",
		DashboardTemplatePath:       "/opt/appdynamics/templates/cluster-template.json",
		DashboardSuffix:             "SUMMARY",
		DashboardDelayMin:           2,
		DeploysToDashboard:          []string{},
		InstrumentationMethod:       "none",
		DefaultInstrumentationTech:  "java",
		BiqService:                  "none",
		InstrumentContainer:         "first",
		InstrumentMatchString:       []string{},
		InitContainerDir:            "/opt/temp",
		AgentLabel:                  "appd-agent",
		AgentUserOverride:           "",
		AgentLogOverride:            "",
		AgentEnvVar:                 "JAVA_OPTS",
		AppNameLiteral:              "",
		AppDAppLabel:                "appd-app",
		AppDTierLabel:               "appd-tier",
		AppDAnalyticsLabel:          "appd-biq",
		AgentMountName:              "appd-agent-repo",
		AgentMountPath:              "/opt/appdynamics",
		AppLogMountName:             "appd-volume",
		AppLogMountPath:             "/opt/appdlogs",
		JDKMountName:                "jdk-repo",
		JDKMountPath:                "$JAVA_HOME/lib",
		AnalyticsAgentUrl:           "http://analytics-proxy:9090",
		AnalyticsAgentContainerName: "appd-analytics-agent",
		AppDInitContainerName:       "appd-agent-attach",
		AnalyticsAgentImage:         "docker.io/appdynamics/analytics-agent:latest",
		AppDJavaAttachImage:         "docker.io/appdynamics/java-agent:latest",
		AppDDotNetAttachImage:       "docker.io/appdynamics/dotnet-core-agent:latest",
		NsToMonitor:                 []string{},
		NsToMonitorExclude:          []string{},
		NodesToMonitor:              []string{},
		NodesToMonitorExclude:       []string{},
		NsToInstrument:              []string{},
		NsToInstrumentExclude:       []string{},
		NSInstrumentRule:            []AgentRequest{},
		InitRequestMem:              "50",
		InitRequestCpu:              "0.1",
		BiqRequestMem:               "600",
		BiqRequestCpu:               "0.1",
		NetVizPort:                  0,
		ProxyUrl:                    "",
		ProxyUser:                   "",
		ProxyPass:                   "",
		LogLines:                    0, //0 - no logging}
		PodEventNumber:              1,
		LogLevel:                    "info",
		OverconsumptionThreshold:    80,
		InstrumentationUpdated:      false,
	}

	return &bag
}
