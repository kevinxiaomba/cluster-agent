package models

import (
	"github.com/fatih/structs"
	"k8s.io/api/core/v1"
)

type RQSchema struct {
	Name      string
	Namespace string
	Selector  string
	PodsOnly  bool
	Spec      RQFields
	Used      RQFields
}

type RQFields struct {
	Name          string
	Namespace     string
	RequestCpu    int64
	RequestMemory int64
	LimitCpu      int64
	LimitMemory   int64
	StoragePod    int64
	Storage       int64
	PVC           int64
	Pods          int64
	Path          string
	Metadata      map[string]AppDMetricMetadata
}

func NewRQFields() RQFields {
	return RQFields{RequestCpu: 0, RequestMemory: 0, LimitCpu: 0, LimitMemory: 0, StoragePod: 0, Storage: 0, PVC: 0, Pods: 0}
}

func (cpm RQFields) GetPath() string {

	return cpm.Path
}

func (cpm RQFields) ShouldExcludeField(fieldName string) bool {
	if fieldName == "Path" || fieldName == "Metadata" || fieldName == "Name" || fieldName == "Namespace" {
		return true
	}
	return false
}

func (cpm RQFields) Unwrap() *map[string]interface{} {
	objMap := structs.Map(cpm)

	return &objMap
}

func (self RQFields) Increment(rqVals *RQFields) {
	self.RequestCpu += rqVals.RequestCpu
	self.RequestMemory += rqVals.RequestMemory
	self.LimitCpu += rqVals.LimitCpu
	self.LimitMemory += rqVals.LimitMemory
	self.StoragePod += rqVals.StoragePod
	self.Storage += rqVals.Storage
	self.PVC += rqVals.PVC
	self.Pods += rqVals.Pods
}

func NewRQ(rq *v1.ResourceQuota) RQSchema {
	rqSchema := RQSchema{}
	rqSchema.Name = rq.Name
	rqSchema.Namespace = rq.Namespace
	//TODO: incorporate scope selector

	rqSchema.Spec = RQFields{Name: rqSchema.Name, Namespace: rqSchema.Namespace}
	rqSchema.Used = RQFields{Name: rqSchema.Name, Namespace: rqSchema.Namespace}

	//resource limits don't apply to BestEffort scope
	rqSchema.PodsOnly = len(rq.Spec.Scopes) == 1 && rq.Spec.Scopes[0] == v1.ResourceQuotaScopeBestEffort

	if rq.Spec.Hard.Cpu() != nil {
		rqSchema.Spec.RequestCpu = rq.Spec.Hard.Cpu().MilliValue()
	}

	if rq.Spec.Hard.Memory() != nil {
		rqSchema.Spec.RequestMemory = rq.Spec.Hard.Memory().MilliValue() / 1000
	}

	if rq.Spec.Hard.Pods() != nil {
		rqSchema.Spec.Pods = rq.Spec.Hard.Pods().MilliValue()
	}

	if rq.Spec.Hard.StorageEphemeral() != nil {
		rqSchema.Spec.StoragePod = rq.Spec.Hard.StorageEphemeral().MilliValue()
	}
	for key, val := range rq.Spec.Hard {
		if key == "limits.cpu" {
			rqSchema.Spec.LimitCpu = val.MilliValue()
		}

		if key == "requests.cpu" {
			rqSchema.Spec.RequestCpu = val.MilliValue()
		}

		if key == "limits.memory" {
			rqSchema.Spec.LimitMemory = val.MilliValue()
		}

		if key == "requests.memory" {
			rqSchema.Spec.RequestMemory = val.MilliValue()
		}

		if key == "requests.storage" {
			rqSchema.Spec.Storage = val.MilliValue()
		}

		if key == "persistentvolumeclaims" {
			rqSchema.Spec.PVC = val.MilliValue()
		}
	}

	//used

	if rq.Status.Used.Cpu() != nil {
		rqSchema.Used.RequestCpu = rq.Status.Used.Cpu().MilliValue()
	}

	if rq.Status.Used.Memory() != nil {
		rqSchema.Used.RequestMemory = rq.Status.Used.Memory().MilliValue() / 1000
	}

	if rq.Status.Used.Pods() != nil {
		rqSchema.Used.Pods = rq.Status.Used.Pods().MilliValue()
	}

	if rq.Status.Used.StorageEphemeral() != nil {
		rqSchema.Used.StoragePod = rq.Status.Used.StorageEphemeral().MilliValue()
	}

	for key, val := range rq.Status.Used {
		if key == "limits.cpu" {
			rqSchema.Used.LimitCpu = val.MilliValue()
		}

		if key == "requests.cpu" {
			rqSchema.Used.RequestCpu = val.MilliValue()
		}

		if key == "limits.memory" {
			rqSchema.Used.LimitMemory = val.MilliValue()
		}

		if key == "requests.memory" {
			rqSchema.Used.RequestMemory = val.MilliValue()
		}

		if key == "requests.storage" {
			rqSchema.Used.Storage = val.MilliValue()
		}

		if key == "persistentvolumeclaims" {
			rqSchema.Used.PVC = val.MilliValue()
		}
	}

	return rqSchema
}

func (rq *RQSchema) AppliesToPod(podObject *PodSchema) bool {
	check := !rq.PodsOnly && rq.Namespace == podObject.Namespace

	return check
}

func (rq *RQSchema) AddQuotaStatsToNamespaceMetrics(metrics *ClusterPodMetrics) {
	metrics.QuotasSpec = rq.Spec
	metrics.QuotasUsed = rq.Used
}

func (rq *RQSchema) IncrementQuotaStatsClusterMetrics(metrics *ClusterPodMetrics) {
	metrics.QuotasSpec.Increment(&rq.Spec)
	metrics.QuotasUsed.Increment(&rq.Used)
}

func (rq *RQSchema) AddQuotaStatsToAppMetrics(metrics *ClusterAppMetrics) {
	metrics.QuotasSpec = rq.Spec
	metrics.QuotasUsed = rq.Used
}
