package models

import (
	"fmt"
	"reflect"
	"time"

	"github.com/fatih/structs"
	"k8s.io/api/core/v1"
)

type PodSchemaDefWrapper struct {
	Schema PodSchemaDef `json:"schema"`
}

type PodSchemaDef struct {
	Name                          string `json:"name"`
	Namespace                     string `json:"namespace"`
	ClusterName                   string `json:"clusterName"`
	Labels                        string `json:"labels"`
	Annotations                   string `json:"annotations"`
	ContainerCount                string `json:"containerCount"`
	InitContainerCount            string `json:"initContainerCount"`
	NodeName                      string `json:"nodeName"`
	Priority                      string `json:"priority"`
	RestartPolicy                 string `json:"restartPolicy"`
	ServiceAccountName            string `json:"serviceAccountName"`
	TerminationGracePeriodSeconds string `json:"terminationGracePeriodSeconds"`
	Tolerations                   string `json:"tolerations"`
	NodeAffinityPreferred         string `json:"nodeAffinityPreferred"`
	NodeAffinityRequired          string `json:"nodeAffinityRequired"`
	PodAffinityPreferred          string `json:"podAffinityPreferred"`
	PodAffinityRequired           string `json:"podAffinityRequired"`
	PodAntiAffinityPreferred      string `json:"podAntiAffinityPreferred"`
	PodAntiAffinityRequired       string `json:"podAntiAffinityRequired"`
	HostIP                        string `json:"hostIP"`
	Phase                         string `json:"phase"`
	PodIP                         string `json:"podIP"`
	Reason                        string `json:"reason"`
	StartTime                     string `json:"startTime"`
	LastTransitionTimeCondition   string `json:"lastTransitionTimeCondition"`
	ReasonCondition               string `json:"reasonCondition"`
	StatusCondition               string `json:"statusCondition"`
	TypeCondition                 string `json:"typeCondition"`
	LimitsDefined                 string `json:"limitsDefined"`
	LiveProbes                    string `json:"liveProbes"`
	ReadyProbes                   string `json:"readyProbes"`
	PodRestarts                   string `json:"podRestarts"`
	NumPrivileged                 string `json:"numPrivileged"`
	Ports                         string `json:"ports"`
	MemRequest                    string `json:"memRequest"`
	CpuRequest                    string `json:"cpuRequest"`
	CpuLimit                      string `json:"cpuLimit"`
	MemLimit                      string `json:"memLimit"`
	PodStorageRequest             string `json:"podStorageRequest"`
	PodStorageLimit               string `json:"podStorageLimit"`
	StorageRequest                string `json:"storageRequest"`
	StorageCapacity               string `json:"storageCapacity"`
	CpuUse                        string `json:"cpuUse"`
	MemUse                        string `json:"memUse"`
	Images                        string `json:"images"`
	WaitReasons                   string `json:"waitReasons"`
	TermReasons                   string `json:"termReasons"`
	RunningStartTime              string `json:"runningStartTime"`
	TerminationTime               string `json:"terminationTime"`
}

func NewPodSchemaDefWrapper() PodSchemaDefWrapper {
	schema := NewPodSchemaDef()
	wrapper := PodSchemaDefWrapper{Schema: schema}
	return wrapper
}

func (sd PodSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

func NewPodSchemaDef() PodSchemaDef {
	pdsd := PodSchemaDef{Name: "string", Namespace: "string", ClusterName: "string", Labels: "string", Annotations: "string", ContainerCount: "integer",
		InitContainerCount: "integer", NodeName: "string", Priority: "integer", RestartPolicy: "string", ServiceAccountName: "string", TerminationGracePeriodSeconds: "integer",
		Tolerations: "string", NodeAffinityPreferred: "string", NodeAffinityRequired: "string", PodAffinityPreferred: "string", PodAffinityRequired: "string",
		PodAntiAffinityPreferred: "string", PodAntiAffinityRequired: "string",
		HostIP: "string", Phase: "string", PodIP: "string", Reason: "string", StartTime: "date", LastTransitionTimeCondition: "date", ReasonCondition: "string",
		StatusCondition: "string", TypeCondition: "string", LimitsDefined: "boolean", LiveProbes: "integer", ReadyProbes: "integer", PodRestarts: "integer",
		NumPrivileged: "integer", Ports: "string", MemRequest: "float", CpuRequest: "float", CpuLimit: "float", MemLimit: "float",
		PodStorageRequest: "float", PodStorageLimit: "float", StorageRequest: "float", StorageCapacity: "float", CpuUse: "float", MemUse: "float",
		Images: "string", WaitReasons: "string", TermReasons: "string", RunningStartTime: "date", TerminationTime: "date"}
	return pdsd
}

func (sd PodSchemaDef) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

type PodSchema struct {
	Name                          string                     `json:"name"`
	Namespace                     string                     `json:"namespace"`
	ClusterName                   string                     `json:"clusterName"`
	Labels                        string                     `json:"labels"`
	Annotations                   string                     `json:"annotations"`
	ContainerCount                int                        `json:"containerCount"`
	InitContainerCount            int                        `json:"initContainerCount"`
	NodeName                      string                     `json:"nodeName"`
	Priority                      int32                      `json:"priority"`
	RestartPolicy                 string                     `json:"restartPolicy"`
	ServiceAccountName            string                     `json:"serviceAccountName"`
	TerminationGracePeriodSeconds int64                      `json:"terminationGracePeriodSeconds"`
	Tolerations                   string                     `json:"tolerations"`
	NodeAffinityPreferred         string                     `json:"nodeAffinityPreferred"`
	NodeAffinityRequired          string                     `json:"nodeAffinityRequired"`
	PodAffinityPreferred          string                     `json:"podAffinityPreferred"`
	PodAffinityRequired           string                     `json:"podAffinityRequired"`
	PodAntiAffinityPreferred      string                     `json:"podAntiAffinityPreferred"`
	PodAntiAffinityRequired       string                     `json:"podAntiAffinityRequired"`
	HostIP                        string                     `json:"hostIP"`
	Phase                         string                     `json:"phase"`
	PodIP                         string                     `json:"podIP"`
	Reason                        string                     `json:"reason"`
	StartTime                     time.Time                  `json:"startTime"`
	LastTransitionTimeCondition   *time.Time                 `json:"lastTransitionTimeCondition"`
	ReasonCondition               string                     `json:"reasonCondition"`
	StatusCondition               string                     `json:"statusCondition"`
	TypeCondition                 string                     `json:"typeCondition"`
	LimitsDefined                 bool                       `json:"limitsDefined"`
	LiveProbes                    int                        `json:"liveProbes"`
	ReadyProbes                   int                        `json:"readyProbes"`
	PodRestarts                   int32                      `json:"podRestarts"`
	NumPrivileged                 int                        `json:"numPrivileged"`
	Ports                         string                     `json:"ports"`
	MemRequest                    int64                      `json:"memRequest"`
	CpuRequest                    int64                      `json:"cpuRequest"`
	CpuLimit                      int64                      `json:"cpuLimit"`
	MemLimit                      int64                      `json:"memLimit"`
	PodStorageRequest             int64                      `json:"podStorageRequest"`
	PodStorageLimit               int64                      `json:"podStorageLimit"`
	StorageRequest                int64                      `json:"storageRequest"`
	StorageCapacity               int64                      `json:"storageCapacity"`
	CpuUse                        int64                      `json:"cpuUse"`
	MemUse                        int64                      `json:"memUse"`
	Images                        string                     `json:"images"`
	WaitReasons                   string                     `json:"waitReasons"`
	TermReasons                   string                     `json:"termReasons"`
	RunningStartTime              *time.Time                 `json:"runningStartTime"`
	TerminationTime               *time.Time                 `json:"terminationTime"`
	PendingTime                   int64                      `json:"-"`
	Containers                    map[string]ContainerSchema `json:"-"`
	InitContainers                map[string]ContainerSchema `json:"-"`
	Owner                         string                     `json:"-"`
	IsEvicted                     bool                       `json:"-"`
	AppID                         int                        `json:"-"`
	TierID                        int                        `json:"-"`
	NodeID                        int                        `json:"-"`
	AppName                       string                     `json:"-"`
	TierName                      string                     `json:"-"`
	APMNodeName                   string                     `json:"-"`
	Services                      []ServiceSchema            `json:"-"`
	Endpoints                     []v1.Endpoints             `json:"-"`
	StartTimeMillis               int64                      `json:"-"`
	RunningStartTimeMillis        int64                      `json:"-"`
	TerminationTimeMillis         int64                      `json:"-"`
	UpTimeMillis                  int64                      `json:"-"`
	BreakPointMillis              int64                      `json:"-"` //time when a container exited
	MissingDependencies           bool                       `json:"-"`
	NoConnectivity                bool                       `json:"-"`
}

type PodObjList struct {
	Items []PodSchema
}

func (ps *PodSchema) Equals(obj *PodSchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

//func (ps *PodSchema) GetPodKey() string {
//	return fmt.Sprintf("%s/%s", ps.Namespace, ps.Name)
//}

func NewPodObjList() PodObjList {
	return PodObjList{}
}

func NewPodObj() PodSchema {
	return PodSchema{PendingTime: 0, UpTimeMillis: 0, RunningStartTimeMillis: 0, TerminationTimeMillis: 0, Services: []ServiceSchema{}, Endpoints: []v1.Endpoints{}}
}

func (p PodSchema) ToString() string {
	return fmt.Sprintf("Name: %s\n Namespace: %s\n ClusterName: %s\n Labels: %s\n Annotations: %s\n ContainerCount: %d\n InitContainerCount: %d\n"+
		"NodeName: %s\n Priority: %d\n RestartPolicy: %s\n ServiceAccountName: %s\n TerminationGracePeriodSeconds: %d\n Tolerations: %s\n"+
		"NodeAffinityPreferred: %s\n NodeAffinityRequired: %s\n HasPodAffinity: %t\n HasPodAntiAffinity: %t\n HostIP: %s\n Phase: %s\n PodIP: %s\n"+
		"Reason: %s\n StartTime: %s\n LastTransitionTimeCondition: %s\n ReasonCondition: %s\n StatusCondition: %s\n TypeCondition: %s\n"+
		"LimitsDefined: %t\n LiveProbes: %d\n ReadyProbes: %d\n PodRestarts: %d\n NumPrivileged: %d\n Ports: %s\n MemRequest: %d\n CpuRequest: %d\n"+
		"CpuLimit: %d\n MemLimit:%d\n CpuUse: %d\n MemUse: %d\n Images: %s\n WaitReasons: %s\n TermReasons: %s\n RunningStartTime: %s\n"+
		"TerminationTime: %s\n PendingTime: %d\n InitTime: %s\n",
		p.Name, p.Namespace, p.ClusterName, p.Labels, p.Annotations, p.ContainerCount, p.InitContainerCount, p.NodeName,
		p.Priority, p.RestartPolicy, p.ServiceAccountName, p.TerminationGracePeriodSeconds, p.Tolerations, p.NodeAffinityPreferred,
		p.NodeAffinityRequired, p.PodAffinityPreferred, p.PodAffinityRequired, p.PodAntiAffinityPreferred, p.PodAntiAffinityRequired, p.HostIP, p.Phase, p.PodIP, p.Reason, p.StartTime.String(),
		p.LastTransitionTimeCondition, p.ReasonCondition, p.StatusCondition, p.TypeCondition, p.LimitsDefined, p.LiveProbes, p.ReadyProbes,
		p.PodRestarts, p.NumPrivileged, p.Ports, p.MemRequest, p.CpuRequest, p.CpuLimit, p.MemLimit, p.CpuUse, p.MemUse,
		p.Images, p.WaitReasons, p.TermReasons, p.RunningStartTime.String(), p.TerminationTime.String(), p.PendingTime)
}

func (p PodSchema) GetState() string {
	if p.IsEvicted {
		return "Evicted"
	} else {
		return p.Phase
	}
}

func (l PodObjList) AddItem(obj PodSchema) []PodSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l PodObjList) Clear() []PodSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}
