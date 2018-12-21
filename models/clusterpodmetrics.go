package models

import (
	"fmt"
)

type ClusterPodMetrics struct {
	Path               string
	Metadata           map[string]AppDMetricMetadata
	Namespace          string
	Nodename           string
	PodCount           int64
	Evictions          int64
	PodRestarts        int64
	PodRunning         int64
	PodFailed          int64
	PodPending         int64
	PendingTime        int64
	ContainerCount     int64
	InitContainerCount int64
	NoLimits           int64
	NoReadinessProbe   int64
	NoLivenessProbe    int64
	Privileged         int64
	HasTolerations     int64
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

func (cpm ClusterPodMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterPodMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "Nodename" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" {
		return true
	}
	return false
}

func NewClusterPodMetrics(bag *AppDBag, ns string, node string) ClusterPodMetrics {
	p := RootPath
	if node != "" && node != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NODES, METRIC_SEPARATOR, node, METRIC_SEPARATOR)
	} else if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR)
	}
	return ClusterPodMetrics{Namespace: ns, Nodename: node, PodCount: 0, Evictions: 0,
		PodRestarts: 0, PodRunning: 0, PodFailed: 0, PodPending: 0, PendingTime: 0, ContainerCount: 0, InitContainerCount: 0,
		NoLimits: 0, NoReadinessProbe: 0, NoLivenessProbe: 0, Privileged: 0, HasTolerations: 0,
		HasNodeAffinity: 0, HasPodAffinity: 0, HasPodAntiAffinity: 0, RequestCpu: 0, RequestMemory: 0, LimitCpu: 0, LimitMemory: 0,
		UseCpu: 0, UseMemory: 0, Path: p}
}

func NewClusterPodMetricsMetadata(bag *AppDBag, ns string, node string) ClusterPodMetrics {
	metrics := NewClusterPodMetrics(bag, ns, node)
	metrics.Metadata = buildAppMetadata(bag)
	return metrics
}

func buildAppMetadata(bag *AppDBag) map[string]AppDMetricMetadata {
	pathBase := "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"

	meta := make(map[string]AppDMetricMetadata, 22)
	path := pathBase + "PodCount"
	meta[path] = NewAppDMetricMetadata("PodCount", bag.PodSchemaName, path, "select * from "+bag.PodSchemaName)

	return meta
}
