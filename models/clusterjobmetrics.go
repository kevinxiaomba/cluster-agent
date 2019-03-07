package models

import (
	"fmt"

	"github.com/fatih/structs"
)

type ClusterJobMetrics struct {
	Path            string
	Metadata        map[string]AppDMetricMetadata
	Namespace       string
	JobCount        int64
	JobActiveCount  int64
	JobSuccessCount int64
	JobFailedCount  int64
	JobDuration     int64
}

func (cpm ClusterJobMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func NewClusterJobMetrics(bag *AppDBag, ns string, node string) ClusterJobMetrics {
	p := RootPath
	if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR)
	}
	return ClusterJobMetrics{Namespace: ns, JobCount: 0, JobActiveCount: 0,
		JobSuccessCount: 0, JobFailedCount: 0, JobDuration: 0, Path: p}
}

func NewClusterJobMetricsMetadata(bag *AppDBag, ns string, node string) ClusterJobMetrics {
	metrics := NewClusterJobMetrics(bag, ns, node)
	metrics.Metadata = buildJobMetadata(bag)
	return metrics
}

func buildJobMetadata(bag *AppDBag) map[string]AppDMetricMetadata {
	pathBase := "Application Infrastructure Performance|%s|Custom Metrics|Cluster Stats|"

	meta := make(map[string]AppDMetricMetadata, 22)
	path := pathBase + "JobCount"
	meta[path] = NewAppDMetricMetadata("JobCount", bag.PodSchemaName, path, "select * from "+bag.JobSchemaName)

	return meta
}
