package models

import (
	"fmt"
	"strconv"
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
	PortMetrics   []ClusterInstancePortMetrics
}

type ClusterInstancePortMetrics struct {
	Path          string
	Namespace     string
	TierName      string
	ContainerName string
	PodName       string
	PortName      string
	Mapped        int64 //0 or 1
	Ready         int64 // 0 or 1
}

func (cpm ClusterInstanceMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterInstancePortMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterInstanceMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "ContainerName" ||
		fieldName == "InstanceIndex" || fieldName == "PodName" || fieldName == "PortMetrics" {
		return true
	}
	return false
}

func (cpm ClusterInstancePortMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "TierName" || fieldName == "Namespace" || fieldName == "Path" || fieldName == "Metadata" || fieldName == "ContainerName" ||
		fieldName == "InstanceIndex" || fieldName == "PodName" || fieldName == "PortName" {
		return true
	}
	return false
}

func NewClusterInstanceMetrics(bag *AppDBag, podObject *PodSchema, containerName string) ClusterInstanceMetrics {
	p := RootPath
	p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, podObject.Namespace, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, podObject.Owner, METRIC_SEPARATOR, METRIC_PATH_CONT, METRIC_SEPARATOR, containerName, METRIC_SEPARATOR, METRIC_PATH_INSTANCES, METRIC_SEPARATOR, podObject.Name, METRIC_SEPARATOR)
	im := ClusterInstanceMetrics{Namespace: podObject.Namespace, TierName: podObject.Owner, ContainerName: containerName, PodName: podObject.Name, APMNodeID: 0, Restarts: 0, UseCpu: 0, UseMemory: 0, Path: p, PortMetrics: []ClusterInstancePortMetrics{}}
	for _, c := range podObject.Containers {
		for _, cp := range c.ContainerPorts {
			name := strconv.Itoa(int(cp.PortNumber))
			portMetrics := NewClusterInstancePortMetrics(bag, im.Namespace, im.TierName, im.ContainerName, im.PodName, name, cp.Mapped, cp.Ready)
			im.PortMetrics = append(im.PortMetrics, portMetrics)
		}
	}
	return im
}

func NewClusterInstancePortMetrics(bag *AppDBag, ns string, tierName string, containerName string, podName string, portName string, mapped, ready bool) ClusterInstancePortMetrics {
	p := RootPath
	p = fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR, METRIC_PATH_APPS, METRIC_SEPARATOR, tierName, METRIC_SEPARATOR, METRIC_PATH_CONT, METRIC_SEPARATOR, containerName, METRIC_SEPARATOR, METRIC_PATH_INSTANCES, METRIC_SEPARATOR, podName, METRIC_SEPARATOR, METRIC_PATH_PORTS, METRIC_SEPARATOR, portName, METRIC_SEPARATOR)

	portMetrics := ClusterInstancePortMetrics{Namespace: ns, TierName: tierName, ContainerName: containerName, PodName: podName, PortName: portName, Mapped: 0, Ready: 0, Path: p}
	if mapped {
		portMetrics.Mapped = 1
	}
	if ready {
		portMetrics.Ready = 1
	}
	return portMetrics
}

func NewClusterInstanceMetricsMetadata(bag *AppDBag, podObject *PodSchema, containerName string) ClusterInstanceMetrics {
	metrics := NewClusterInstanceMetrics(bag, podObject, containerName)
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
