package models

import (
	"fmt"
	"time"
)

type PodSchema struct {
	Name                          string
	Namespace                     string
	ClusterName                   string
	Labels                        string
	Annotations                   string
	ContainerCount                int
	InitContainerCount            int
	NodeName                      string
	Priority                      int32
	RestartPolicy                 string
	ServiceAccountName            string
	TerminationGracePeriodSeconds int64
	Tolerations                   string
	NodeAffinityPreferred         string
	NodeAffinityRequired          string
	HasPodAffinity                bool
	HasPodAntiAffinity            bool
	HostIP                        string
	Phase                         string
	PodIP                         string
	Reason                        string
	StartTime                     time.Time
	LastTransitionTimeCondition   time.Time
	ReasonCondition               string
	StatusCondition               string
	TypeCondition                 string
	LimitsDefined                 bool
	LiveProbes                    int
	ReadyProbes                   int
	PodRestarts                   int32
	NumPrivileged                 int
	Ports                         string
	MemRequest                    int64
	CpuRequest                    int64
	CpuLimit                      int64
	MemLimit                      int64
	CpuUse                        int64
	MemUse                        int64
	Images                        string
	WaitReasons                   string
	TermReasons                   string
	RunningStartTime              time.Time
	TerminationTime               time.Time
	Mounts                        string
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
