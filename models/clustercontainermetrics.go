package models

import (
	"fmt"
)

type ClusterContainerMetrics struct {
	Path             string
	Metadata         map[string]AppDMetricMetadata
	Namespace        string
	TierName         string
	ContainerName    string
	Restarts         int64
	ContainerCount   int64
	RequestCpu       int64
	RequestMemory    int64
	LimitCpu         int64
	LimitMemory      int64
	NoLimits         int64
	NoReadinessProbe int64
	NoLivenessProbe  int64
}

func (cpm ClusterContainerMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterContainerMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "ContainerName" ||
		fieldName == "InstanceIndex" || fieldName == "PodName" {
		return true
	}
	return false
}

func NewClusterContainerMetrics(bag *AppDBag, ns string, tierName string, containerName string) ClusterContainerMetrics {
	p := RootPath
	p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, tierName, METRIC_SEPARATOR, METRIC_PATH_CONT, METRIC_SEPARATOR, containerName, METRIC_SEPARATOR)
	return ClusterContainerMetrics{Namespace: ns, TierName: tierName, ContainerName: containerName, Restarts: 0,
		RequestCpu: 0, RequestMemory: 0, LimitCpu: 0, LimitMemory: 0, NoLimits: 0, NoReadinessProbe: 0, NoLivenessProbe: 0, Path: p}
}

func NewClusterContainerMetricsMetadata(bag *AppDBag, ns string, tierName string, containerName string) ClusterContainerMetrics {
	metrics := NewClusterContainerMetrics(bag, ns, tierName, containerName)
	metrics.Metadata = buildContainerMetadata(bag)
	return metrics
}

func buildContainerMetadata(bag *AppDBag) map[string]AppDMetricMetadata {
	pathBase := "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"

	meta := make(map[string]AppDMetricMetadata, 22)
	path := pathBase + "PodCount"
	meta[path] = NewAppDMetricMetadata("PodCount", bag.PodSchemaName, path, "select * from "+bag.PodSchemaName)

	return meta
}
