package models

import (
	"fmt"
)

type ClusterNodeMetrics struct {
	Path                string
	Metadata            map[string]AppDMetricMetadata
	Nodename            string
	ReadyNodes          int64
	OutOfDiskNodes      int64
	MemoryPressureNodes int64
	DiskPressureNodes   int64
	TaintsTotal         int64
	Masters             int64
	Workers             int64
	CapacityMemory      int64
	CapacityCpu         int64
	CapacityPods        int64
	AllocationsMemory   int64
	AllocationsCpu      int64
	UseMemory           int64
	UseCpu              int64
}

func (cpm ClusterNodeMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterNodeMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "Nodename" || fieldName == "Path" || fieldName == "Metadata" {
		return true
	}
	return false
}

func NewClusterNodeMetrics(bag *AppDBag, node string) ClusterNodeMetrics {
	p := RootPath
	if node != "" && node != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NODES, METRIC_SEPARATOR, node, METRIC_SEPARATOR)
	}
	return ClusterNodeMetrics{Nodename: node, ReadyNodes: 0, OutOfDiskNodes: 0, MemoryPressureNodes: 0, DiskPressureNodes: 0,
		TaintsTotal: 0, Masters: 0, Workers: 0, CapacityMemory: 0, CapacityCpu: 0, CapacityPods: 0, AllocationsMemory: 0, AllocationsCpu: 0,
		UseCpu: 0, UseMemory: 0, Path: p}
}

func NewClusterNodeMetricsMetadata(bag *AppDBag, node string) ClusterNodeMetrics {
	metrics := NewClusterNodeMetrics(bag, node)
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
