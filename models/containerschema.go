package models

import (
	"fmt"
	"time"

	"github.com/fatih/structs"
)

type ContainerSchemaDefWrapper struct {
	Schema ContainerSchemaDef `json:"schema"`
}

type ContainerSchemaDef struct {
	Name              string `json:"name"`
	Init              string `json:"init"`
	Namespace         string `json:"namespace"`
	ClusterName       string `json:"clusterName"`
	NodeName          string `json:"nodeName"`
	PodName           string `json:"podName"`
	PodInitTime       string `json:"podInitTime"`
	StartTime         string `json:"startTime"`
	LiveProbes        string `json:"liveProbes"`
	ReadyProbes       string `json:"readyProbes"`
	Restarts          string `json:"restarts"`
	Privileged        string `json:"privileged"`
	Ports             string `json:"ports"`
	MemRequest        string `json:"memRequest"`
	CpuRequest        string `json:"cpuRequest"`
	CpuLimit          string `json:"cpuLimit"`
	MemLimit          string `json:"memLimit"`
	PodStorageRequest string `json:"podStorageRequest"`
	PodStorageLimit   string `json:"podStorageLimit"`
	StorageRequest    string `json:"storageRequest"`
	StorageCapacity   string `json:"storageCapacity"`
	CpuUse            string `json:"cpuUse"`
	MemUse            string `json:"memUse"`
	Image             string `json:"image"`
	WaitReason        string `json:"waitReason"`
	TermReason        string `json:"termReason"`
	TerminationTime   string `json:"terminationTime"`
	Mounts            string `json:"mounts"`
	MissingConfigs    string `json:"missingConfigs"`
	MissingSecrets    string `json:"missingSecrets"`
	MissingServices   string `json:"missingServices"`
	ConsumptionCpu    string `json:"consumptionCpu"`
	ConsumptionMem    string `json:"consumptionMem"`
}

func (sd ContainerSchemaDef) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

func NewContainerSchemaDefWrapper() ContainerSchemaDefWrapper {
	schema := NewContainerSchemaDef()
	wrapper := ContainerSchemaDefWrapper{Schema: schema}
	return wrapper
}

func (sd ContainerSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

func NewContainerSchemaDef() ContainerSchemaDef {
	pdsd := ContainerSchemaDef{Name: "string", Init: "boolean", Namespace: "string", ClusterName: "string", NodeName: "string", PodName: "string", PodInitTime: "date", StartTime: "date", LiveProbes: "integer", ReadyProbes: "integer", Restarts: "integer",
		Privileged: "integer", Ports: "string", MemRequest: "float", CpuRequest: "float", CpuLimit: "float", MemLimit: "float",
		ConsumptionCpu: "float", ConsumptionMem: "float", PodStorageRequest: "float", PodStorageLimit: "float", StorageRequest: "float", StorageCapacity: "float", CpuUse: "float", MemUse: "float",
		Image: "string", WaitReason: "string", TermReason: "string", TerminationTime: "date", Mounts: "string", MissingConfigs: "string",
		MissingSecrets: "string", MissingServices: "string"}
	return pdsd
}

type ContainerPort struct {
	Name       string
	PortNumber int32
	Mapped     bool
	Ready      bool
}

type ContainerSchema struct {
	Name                 string          `json:"name"`
	Init                 bool            `json:"init"`
	Namespace            string          `json:"namespace"`
	ClusterName          string          `json:"clusterName"`
	NodeName             string          `json:"nodeName"`
	PodName              string          `json:"podName"`
	PodInitTime          time.Time       `json:"podInitTime"`
	StartTime            *time.Time      `json:"startTime"`
	LiveProbes           int             `json:"liveProbes"`
	ReadyProbes          int             `json:"readyProbes"`
	Restarts             int32           `json:"restarts"`
	Privileged           int             `json:"privileged"`
	Ports                string          `json:"ports"`
	MemRequest           int64           `json:"memRequest"`
	CpuRequest           int64           `json:"cpuRequest"`
	CpuLimit             int64           `json:"cpuLimit"`
	MemLimit             int64           `json:"memLimit"`
	PodStorageRequest    int64           `json:"podStorageRequest"`
	PodStorageLimit      int64           `json:"podStorageLimit"`
	StorageRequest       int64           `json:"storageRequest"`
	StorageCapacity      int64           `json:"storageCapacity"`
	CpuUse               int64           `json:"cpuUse"`
	MemUse               int64           `json:"memUse"`
	Image                string          `json:"image"`
	WaitReason           string          `json:"waitReason"`
	TermReason           string          `json:"termReason"`
	TerminationTime      time.Time       `json:"terminationTime"`
	Mounts               string          `json:"mounts"`
	MissingConfigs       string          `json:"missingConfigs"`
	MissingSecrets       string          `json:"missingSecrets"`
	MissingServices      string          `json:"missingServices"`
	ConsumptionCpu       float64         `json:"consumptionCpu"`
	ConsumptionMem       float64         `json:"consumptionMem"`
	Index                int8            `json:"-"`
	ContainerPorts       []ContainerPort `json:"-"`
	LastTerminationTime  *time.Time      `json:"-"`
	ExitCode             int32           `json:"-"`
	LimitsDefined        bool            `json:"-"`
	RequestCpuString     string          `json:"-"`
	RequestMemString     string          `json:"-"`
	LimitCpuString       string          `json:"-"`
	LimitMemString       string          `json:"-"`
	ConsumptionCpuString string          `json:"-"`
	ConsumptionMemString string          `json:"-"`
}

type ContainerObjList struct {
	Items []ContainerSchema
}

func NewContainerObjList() ContainerObjList {
	return ContainerObjList{}
}

func NewContainerObj() ContainerSchema {
	return ContainerSchema{ContainerPorts: []ContainerPort{}, ExitCode: 0}
}

func (p ContainerSchema) ToString() string {
	return fmt.Sprintf("Name: %s\n Init: %t\n Namespace: %s\n ClusterName: %s\n "+
		"NodeName: %s\n PodName: %s\n PodInitTime: %s\n StartTime: %s\n LiveProbes: %d\n ReadyProbes: %d\n Restarts: %d\n Privileged: %d\n Ports: %s\n MemRequest: %d\n CpuRequest: %d\n"+
		"CpuLimit: %d\n MemLimit:%d\n CpuUse: %d\n MemUse: %d\n Image: %s\n WaitReason: %s\n TermReason: %s\n "+
		"TerminationTime: %s\n Mounts: %s\n ",
		p.Name, p.Init, p.Namespace, p.ClusterName, p.NodeName,
		p.PodName, p.PodInitTime, p.StartTime.String(), p.LiveProbes, p.ReadyProbes,
		p.Restarts, p.Privileged, p.Ports, p.MemRequest, p.CpuRequest, p.CpuLimit, p.MemLimit, p.CpuUse, p.MemUse,
		p.Image, p.WaitReason, p.TermReason, p.TerminationTime.String(), p.Mounts)
}

func (p *ContainerSchema) HasMissingDependencies() bool {
	return p.MissingConfigs != "" || p.MissingSecrets != ""
}

func (p *ContainerSchema) NoConnectivity() bool {
	return p.MissingServices != ""
}

func (l ContainerObjList) AddItem(obj ContainerSchema) []ContainerSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l ContainerObjList) Clear() []ContainerSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}
