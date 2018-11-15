package models

import (
	"fmt"
	"time"
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
	ContainerCount                string `json:"name"`
	InitContainerCount            string `json:"name"`
	NodeName                      string `json:"name"`
	Priority                      string `json:"name"`
	RestartPolicy                 string `json:"name"`
	ServiceAccountName            string `json:"name"`
	TerminationGracePeriodSeconds string `json:"name"`
	Tolerations                   string `json:"name"`
	NodeAffinityPreferred         string `json:"name"`
	NodeAffinityRequired          string `json:"name"`
	HasPodAffinity                string `json:"name"`
	HasPodAntiAffinity            string `json:"name"`
	HostIP                        string `json:"name"`
	Phase                         string `json:"name"`
	PodIP                         string `json:"name"`
	Reason                        string `json:"name"`
	StartTime                     string `json:"name"`
	LastTransitionTimeCondition   string `json:"name"`
	ReasonCondition               string `json:"name"`
	StatusCondition               string `json:"name"`
	TypeCondition                 string `json:"name"`
	LimitsDefined                 string `json:"name"`
	LiveProbes                    string `json:"name"`
	ReadyProbes                   string `json:"name"`
	PodRestarts                   string `json:"name"`
	NumPrivileged                 string `json:"name"`
	Ports                         string `json:"name"`
	MemRequest                    string `json:"name"`
	CpuRequest                    string `json:"name"`
	CpuLimit                      string `json:"name"`
	MemLimit                      string `json:"name"`
	CpuUse                        string `json:"name"`
	MemUse                        string `json:"name"`
	Images                        string `json:"name"`
	WaitReasons                   string `json:"name"`
	TermReasons                   string `json:"name"`
	RunningStartTime              string `json:"name"`
	TerminationTime               string `json:"name"`
	Mounts                        string `json:"name"`
}

func NewPodSchemaDefWrapper() PodSchemaDefWrapper {
	schema := NewPodSchemaDef()
	wrapper := PodSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewPodSchemaDef() PodSchemaDef {
	pdsd := PodSchemaDef{Name: "string", Namespace: "string", ClusterName: "string", Labels: "string", Annotations: "string", ContainerCount: "integer",
		InitContainerCount: "integer", NodeName: "string", Priority: "string", RestartPolicy: "string", ServiceAccountName: "string", TerminationGracePeriodSeconds: "integer",
		Tolerations: "string", NodeAffinityPreferred: "string", NodeAffinityRequired: "string", HasPodAffinity: "boolean", HasPodAntiAffinity: "boolean",
		HostIP: "string", Phase: "string", PodIP: "string", Reason: "string", StartTime: "date", LastTransitionTimeCondition: "date", ReasonCondition: "string",
		StatusCondition: "string", TypeCondition: "string", LimitsDefined: "boolean", LiveProbes: "integer", ReadyProbes: "integer", PodRestarts: "integer",
		NumPrivileged: "integer", Ports: "string", MemRequest: "float", CpuRequest: "float", CpuLimit: "float", MemLimit: "float", CpuUse: "float", MemUse: "float",
		Images: "string", WaitReasons: "string", TermReasons: "string", RunningStartTime: "date", TerminationTime: "date", Mounts: "string"}
	return pdsd
}

type PodSchema struct {
	Name                          string    `json:"name"`
	Namespace                     string    `json:"namespace"`
	ClusterName                   string    `json:"clusterName"`
	Labels                        string    `json:"labels"`
	Annotations                   string    `json:"annotations"`
	ContainerCount                int       `json:"containerCount"`
	InitContainerCount            int       `json:"initContainerCount"`
	NodeName                      string    `json:"nodeName"`
	Priority                      int32     `json:"priority"`
	RestartPolicy                 string    `json:"restartPolicy"`
	ServiceAccountName            string    `json:"serviceAccountName"`
	TerminationGracePeriodSeconds int64     `json:"terminationGracePeriodSeconds"`
	Tolerations                   string    `json:"tolerations"`
	NodeAffinityPreferred         string    `json:"nodeAffinityPreferred"`
	NodeAffinityRequired          string    `json:"nodeAffinityRequired"`
	HasPodAffinity                bool      `json:"hasPodAffinity"`
	HasPodAntiAffinity            bool      `json:"hasPodAntiAffinity"`
	HostIP                        string    `json:"hostIP"`
	Phase                         string    `json:"phase"`
	PodIP                         string    `json:"podIP"`
	Reason                        string    `json:"reason"`
	StartTime                     time.Time `json:"startTime"`
	LastTransitionTimeCondition   time.Time `json:"lastTransitionTimeCondition"`
	ReasonCondition               string    `json:"eeasonCondition"`
	StatusCondition               string    `json:"statusCondition"`
	TypeCondition                 string    `json:"typeCondition"`
	LimitsDefined                 bool      `json:"limitsDefined"`
	LiveProbes                    int       `json:"liveProbes"`
	ReadyProbes                   int       `json:"readyProbes"`
	PodRestarts                   int32     `json:"podRestarts"`
	NumPrivileged                 int       `json:"numPrivileged"`
	Ports                         string    `json:"ports"`
	MemRequest                    int64     `json:"memRequest"`
	CpuRequest                    int64     `json:"cpuRequest"`
	CpuLimit                      int64     `json:"cpuLimit"`
	MemLimit                      int64     `json:"memLimit"`
	CpuUse                        int64     `json:"cpuUse"`
	MemUse                        int64     `json:"memUse"`
	Images                        string    `json:"images"`
	WaitReasons                   string    `json:"waitReasons"`
	TermReasons                   string    `json:"termReasons"`
	RunningStartTime              time.Time `json:"runningStartTime"`
	TerminationTime               time.Time `json:"terminationTime"`
	Mounts                        string    `json:"mounts"`
}

type PodObjList struct {
	Items []PodSchema
}

func NewPodObjList() PodObjList {
	return PodObjList{}
}

func NewPodObj() PodSchema {
	return PodSchema{}
}

func (p PodSchema) ToString() string {
	return fmt.Sprintf("Name: %s\n Namespace: %s\n ClusterName: %s\n Labels: %s\n Annotations: %s\n ContainerCount: %d\n InitContainerCount: %d\n"+
		"NodeName: %s\n Priority: %d\n RestartPolicy: %s\n ServiceAccountName: %s\n TerminationGracePeriodSeconds: %d\n Tolerations: %s\n"+
		"NodeAffinityPreferred: %s\n NodeAffinityRequired: %s\n HasPodAffinity: %t\n HasPodAntiAffinity: %t\n HostIP: %s\n Phase: %s\n PodIP: %s\n"+
		"Reason: %s\n StartTime: %s\n LastTransitionTimeCondition: %s\n ReasonCondition: %s\n StatusCondition: %s\n TypeCondition: %s\n"+
		"LimitsDefined: %t\n LiveProbes: %d\n ReadyProbes: %d\n PodRestarts: %d\n NumPrivileged: %d\n Ports: %s\n MemRequest: %d\n CpuRequest: %d\n"+
		"CpuLimit: %d\n MemLimit:%d\n CpuUse: %d\n MemUse: %d\n Images: %s\n WaitReasons: %s\n TermReasons: %s\n RunningStartTime: %s\n"+
		"TerminationTime: %s\n Mounts: %s\n",
		p.Name, p.Namespace, p.ClusterName, p.Labels, p.Annotations, p.ContainerCount, p.InitContainerCount, p.NodeName,
		p.Priority, p.RestartPolicy, p.ServiceAccountName, p.TerminationGracePeriodSeconds, p.Tolerations, p.NodeAffinityPreferred,
		p.NodeAffinityRequired, p.HasPodAffinity, p.HasPodAntiAffinity, p.HostIP, p.Phase, p.PodIP, p.Reason, p.StartTime.String(),
		p.LastTransitionTimeCondition, p.ReasonCondition, p.StatusCondition, p.TypeCondition, p.LimitsDefined, p.LiveProbes, p.ReadyProbes,
		p.PodRestarts, p.NumPrivileged, p.Ports, p.MemRequest, p.CpuRequest, p.CpuLimit, p.MemLimit, p.CpuUse, p.MemUse,
		p.Images, p.WaitReasons, p.TermReasons, p.RunningStartTime.String(), p.TerminationTime.String(), p.Mounts)
}

func (l PodObjList) AddItem(obj PodSchema) []PodSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l PodObjList) Clear() []PodSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}
