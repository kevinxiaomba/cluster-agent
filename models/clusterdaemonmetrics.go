package models

import (
	"fmt"

	"github.com/fatih/structs"
)

type ClusterDaemonMetrics struct {
	Path                      string
	Namespace                 string
	DaemonCount               int64
	DaemonReplicasAvailable   int64
	DaemonReplicasUnAvailable int64
	DaemonCollisionCount      int64
	DaemonMissScheduled       int64
}

func (cpm ClusterDaemonMetrics) GetPath() string {

	return cpm.Path
}

func (cpm ClusterDaemonMetrics) ShouldExcludeField(fieldName string) bool {
	if fieldName == "Namespace" || fieldName == "Path" {
		return true
	}
	return false
}

func (cpm ClusterDaemonMetrics) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func NewClusterDaemonMetrics(bag *AppDBag, ns string) ClusterDaemonMetrics {
	p := RootPath
	if ns != "" && ns != ALL {
		p = fmt.Sprintf("%s%s%s%s%s", p, METRIC_PATH_NAMESPACES, METRIC_SEPARATOR, ns, METRIC_SEPARATOR)
	}
	return ClusterDaemonMetrics{Namespace: ns, DaemonCount: 0, DaemonReplicasAvailable: 0, DaemonReplicasUnAvailable: 0, DaemonCollisionCount: 0,
		DaemonMissScheduled: 0, Path: p}
}
