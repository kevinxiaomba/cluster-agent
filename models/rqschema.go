package models

import (
	"github.com/fatih/structs"
	"k8s.io/api/core/v1"
)

type RqSchemaDefWrapper struct {
	Schema RqSchemaDef `json:"schema"`
}

type RqSchemaDef struct {
	ClusterName       string `json:"clusterName"`
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	SpecRequestCpu    string `json:"specRequestCpu"`
	SpecRequestMemory string `json:"specRequestMemory"`
	SpecLimitCpu      string `json:"specLimitCpu"`
	SpecLimitMemory   string `json:"specLimitMemory"`
	SpecStoragePod    string `json:"specStoragePod"`
	SpecStorage       string `json:"specStorage"`
	SpecPVC           string `json:"specPVC"`
	SpecPods          string `json:"specPods"`
	UsedRequestCpu    string `json:"usedRequestCpu"`
	UsedRequestMemory string `json:"usedRequestMemory"`
	UsedLimitCpu      string `json:"usedLimitCpu"`
	UsedLimitMemory   string `json:"usedLimitMemory"`
	UsedStoragePod    string `json:"usedStoragePod"`
	UsedStorage       string `json:"usedStorage"`
	UsedPVC           string `json:"usedPVC"`
	UsedPods          string `json:"usedPods"`
}

func (sd RqSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

func NewRqSchemaDefWrapper() RqSchemaDefWrapper {
	schema := NewRqSchemaDef()
	wrapper := RqSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewRqSchemaDef() RqSchemaDef {
	pdsd := RqSchemaDef{ClusterName: "string", Namespace: "string", Name: "string",
		SpecRequestCpu: "integer", SpecRequestMemory: "integer", SpecLimitCpu: "integer", SpecLimitMemory: "integer",
		SpecStoragePod: "integer", SpecStorage: "integer", SpecPVC: "integer", SpecPods: "integer", UsedRequestCpu: "integer",
		UsedRequestMemory: "integer", UsedLimitCpu: "integer", UsedLimitMemory: "integer", UsedStoragePod: "integer", UsedStorage: "integer",
		UsedPVC: "integer", UsedPods: "integer"}
	return pdsd
}

type RqSchema struct {
	ClusterName       string `json:"clusterName"`
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	SpecRequestCpu    int64  `json:"specRequestCpu"`
	SpecRequestMemory int64  `json:"specRequestMemory"`
	SpecLimitCpu      int64  `json:"specLimitCpu"`
	SpecLimitMemory   int64  `json:"specLimitMemory"`
	SpecStoragePod    int64  `json:"specStoragePod"`
	SpecStorage       int64  `json:"specStorage"`
	SpecPVC           int64  `json:"specPVC"`
	SpecPods          int64  `json:"specPods"`
	UsedRequestCpu    int64  `json:"usedRequestCpu"`
	UsedRequestMemory int64  `json:"usedRequestMemory"`
	UsedLimitCpu      int64  `json:"usedLimitCpu"`
	UsedLimitMemory   int64  `json:"usedLimitMemory"`
	UsedStoragePod    int64  `json:"usedStoragePod"`
	UsedStorage       int64  `json:"usedStorage"`
	UsedPVC           int64  `json:"usedPVC"`
	UsedPods          int64  `json:"usedPods"`
}

func NewRQSchema(rq *v1.ResourceQuota) RqSchema {
	obj := NewRQ(rq)
	schema := RqSchema{}
	schema.ClusterName = rq.ClusterName
	schema.Name = obj.Name
	schema.Namespace = obj.Namespace
	schema.SpecLimitCpu = obj.Spec.LimitCpu
	schema.SpecLimitMemory = obj.Spec.LimitMemory
	schema.SpecPods = obj.Spec.Pods
	schema.SpecPVC = obj.Spec.PVC
	schema.SpecRequestCpu = obj.Spec.RequestCpu
	schema.SpecRequestMemory = obj.Spec.RequestMemory
	schema.SpecStorage = obj.Spec.Storage
	schema.SpecStoragePod = obj.Spec.StoragePod

	schema.UsedLimitCpu = obj.Used.LimitCpu
	schema.UsedLimitMemory = obj.Used.LimitMemory
	schema.UsedPods = obj.Used.Pods
	schema.UsedPVC = obj.Used.PVC
	schema.UsedRequestCpu = obj.Used.RequestCpu
	schema.UsedRequestMemory = obj.Used.RequestMemory
	schema.UsedStorage = obj.Used.Storage
	schema.UsedStoragePod = obj.Used.StoragePod

	return schema
}

type RQSchemaObj struct {
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

func (self *RQFields) Increment(rqVals *RQFields) {
	self.RequestCpu += rqVals.RequestCpu
	self.RequestMemory += rqVals.RequestMemory
	self.LimitCpu += rqVals.LimitCpu
	self.LimitMemory += rqVals.LimitMemory
	self.StoragePod += rqVals.StoragePod
	self.Storage += rqVals.Storage
	self.PVC += rqVals.PVC
	self.Pods += rqVals.Pods
}

func (self *RQFields) Copy(rqVals *RQFields) {
	self.RequestCpu = rqVals.RequestCpu
	self.RequestMemory = rqVals.RequestMemory
	self.LimitCpu = rqVals.LimitCpu
	self.LimitMemory = rqVals.LimitMemory
	self.StoragePod = rqVals.StoragePod
	self.Storage = rqVals.Storage
	self.PVC = rqVals.PVC
	self.Pods = rqVals.Pods
}

func NewRQ(rq *v1.ResourceQuota) RQSchemaObj {
	rqSchema := RQSchemaObj{}
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
		rqSchema.Spec.Pods, _ = rq.Spec.Hard.Pods().AsInt64()
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
		rqSchema.Used.Pods, _ = rq.Status.Used.Pods().AsInt64()
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

func (rq *RQSchemaObj) AppliesToPod(podObject *PodSchema) bool {
	check := !rq.PodsOnly && rq.Namespace == podObject.Namespace

	return check
}

func (rq *RQSchemaObj) AppliesToNamespace(ns string) bool {
	return rq.Namespace == ns
}

func (rq *RQSchemaObj) AddQuotaStatsToNamespaceMetrics(metrics *ClusterPodMetrics) {
	metrics.QuotasSpec.Copy(&rq.Spec)
	metrics.QuotasUsed.Copy(&rq.Used)
}

func (rq *RQSchemaObj) IncrementQuotaStatsClusterMetrics(metrics *ClusterPodMetrics) {
	metrics.QuotasSpec.Increment(&rq.Spec)
	metrics.QuotasUsed.Increment(&rq.Used)
}

func (rq *RQSchemaObj) AddQuotaStatsToAppMetrics(metrics *ClusterAppMetrics) {
	metrics.QuotasSpec.Copy(&rq.Spec)
	metrics.QuotasUsed.Copy(&rq.Used)
}
