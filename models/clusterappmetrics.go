package models

import (
	"fmt"
)

type ClusterAppMetrics struct {
	Path               string
	Metadata           map[string]AppDMetricMetadata
	Namespace          string
	TierName           string
	ContainerName      string
	PodCount           int64
	Evictions          int64
	PodRestarts        int64
	PodRunning         int64
	PodFailed          int64
	PodPending         int64
	PendingTime        int64
	ContainerCount     int64
	InitContainerCount int64
	RequestCpu         int64
	RequestMemory      int64
	LimitCpu           int64
	LimitMemory        int64
	UseCpu             int64
	UseMemory          int64
}

func (cpm ClusterAppMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterAppMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "ContainerName" {
		return true
	}
	return false
}

func NewClusterAppMetrics(bag *AppDBag, ns string, tierName string, containerName string) ClusterAppMetrics {
	p := RootPath
	if containerName != "" && containerName != ALL {
		p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, tierName, METRIC_SEPARATOR, containerName, METRIC_SEPARATOR)
	} else {
		p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, tierName, METRIC_SEPARATOR)
	}
	return ClusterAppMetrics{Namespace: ns, TierName: tierName, ContainerName: containerName, PodCount: 0, Evictions: 0,
		PodRestarts: 0, PodRunning: 0, PodFailed: 0, PodPending: 0, PendingTime: 0, ContainerCount: 0, InitContainerCount: 0,
		RequestCpu: 0, RequestMemory: 0, LimitCpu: 0, LimitMemory: 0, UseCpu: 0, UseMemory: 0, Path: p}
}

func NewClusterAppMetricsMetadata(bag *AppDBag, ns string, tierName string, containerName string) ClusterAppMetrics {
	metrics := NewClusterAppMetrics(bag, ns, tierName, containerName)
	metrics.Metadata = buildMetadata(bag)
	return metrics
}

func buildMetadata(bag *AppDBag) map[string]AppDMetricMetadata {
	pathBase := "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"

	meta := make(map[string]AppDMetricMetadata, 22)
	path := pathBase + "PodCount"
	meta[path] = NewAppDMetricMetadata("PodCount", bag.PodSchemaName, path, "select * from "+bag.PodSchemaName)

	return meta
}
