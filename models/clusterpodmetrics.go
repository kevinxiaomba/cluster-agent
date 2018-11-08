package models

import (
	"fmt"
)

type ClusterPodMetrics struct {
	Path               string
	Metadata           []AppDMetricMetadata
	Namespace          string
	Nodename           string
	PodsCount          int64
	Evictions          int64
	PodRestarts        int64
	PodRunning         int64
	PodFailed          int64
	PodPending         int64
	ContainerCount     int64
	InitContainerCount int64
	NoLimits           int64
	NoReadinessProbe   int64
	NoLivenessProbe    int64
	PrivilegedPods     int64
	TolerationsCount   int64
	HasNodeAffinity    int64
	HasPodAffinity     int64
	HasPodAntiAffinity int64
	RequestCpu         int64
	RequestMemory      int64
	LimitCpu           int64
	LimitMemory        int64
	UseCpu             int64
	UseMemory          int64
}

func NewClusterPodMetrics(ns string, node string) ClusterPodMetrics {
	p := RootPath
	if node != "" && node != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_SEPARATOR, METRIC_PATH_NODES, METRIC_SEPARATOR, node)
	} else if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_SEPARATOR, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns)
	}
	return ClusterPodMetrics{Namespace: ns, Nodename: node, PodsCount: 0, Evictions: 0,
		PodRestarts: 0, PodRunning: 0, PodFailed: 0, PodPending: 0, ContainerCount: 0, InitContainerCount: 0,
		NoLimits: 0, NoReadinessProbe: 0, NoLivenessProbe: 0, PrivilegedPods: 0, TolerationsCount: 0,
		HasNodeAffinity: 0, HasPodAffinity: 0, HasPodAntiAffinity: 0, RequestCpu: 0, RequestMemory: 0, LimitCpu: 0, LimitMemory: 0,
		UseCpu: 0, UseMemory: 0, Path: p, Metadata: buildMetadata()}
}

func buildMetadata() []AppDMetricMetadata {
	meta := make([]AppDMetricMetadata, 22)

	return meta
}
