package models

import (
	"fmt"
)

type ClusterInstanceMetrics struct {
	Path          string
	Metadata      map[string]AppDMetricMetadata
	Namespace     string
	TierName      string
	ContainerName string
	PodName       string
	APMNodeID     int64
	Restarts      int64
	UseCpu        int64
	UseMemory     int64
}

func (cpm ClusterInstanceMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterInstanceMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "ContainerName" ||
		fieldName == "InstanceIndex" || fieldName == "PodName" {
		return true
	}
	return false
}

func NewClusterInstanceMetrics(bag *AppDBag, ns string, tierName string, containerName string, podName string) ClusterInstanceMetrics {
	p := RootPath
	p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, tierName, METRIC_SEPARATOR, METRIC_PATH_CONT, METRIC_SEPARATOR, containerName, METRIC_SEPARATOR, METRIC_PATH_INSTANCES, METRIC_SEPARATOR, podName, METRIC_SEPARATOR)
	return ClusterInstanceMetrics{Namespace: ns, TierName: tierName, ContainerName: containerName, APMNodeID: 0, Restarts: 0, UseCpu: 0, UseMemory: 0, Path: p}
}

func NewClusterInstanceMetricsMetadata(bag *AppDBag, ns string, tierName string, containerName string, podName string) ClusterInstanceMetrics {
	metrics := NewClusterInstanceMetrics(bag, ns, tierName, containerName, podName)
	metrics.Metadata = buildInstanceMetadata(bag)
	return metrics
}

func buildInstanceMetadata(bag *AppDBag) map[string]AppDMetricMetadata {
	pathBase := "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"

	meta := make(map[string]AppDMetricMetadata, 22)
	path := pathBase + "PodCount"
	meta[path] = NewAppDMetricMetadata("PodCount", bag.PodSchemaName, path, "select * from "+bag.PodSchemaName)

	return meta
}
