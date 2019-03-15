package models

import (
	"reflect"

	"time"

	appsv1 "k8s.io/api/apps/v1"
)

type DeploySchemaDefWrapper struct {
	Schema DeploySchemaDef `json:"schema"`
}

type DeploySchemaDef struct {
	Name                  string `json:"name"`
	ClusterName           string `json:"clusterName"`
	Namespace             string `json:"namespace"`
	ObjectUid             string `json:"object_uid"`
	CreationTimestamp     string `json:"creationTimestamp"`
	DeletionTimestamp     string `json:"deletionTimestamp"`
	Labels                string `json:"labels"`
	Annotations           string `json:"annotations"`
	MinReadySecs          string `json:"minReadySecs"`
	ProgressDeadlineSecs  string `json:"progressDeadlineSecs"`
	Replicas              string `json:"replicas"`
	RevisionHistoryLimits string `json:"revisionHistoryLimits"`
	Strategy              string `json:"strategy"`
	MaxSurge              string `json:"maxSurge"`
	MaxUnavailable        string `json:"maxUnavailable"`
	ReplicasAvailable     string `json:"replicasAvailable"`
	ReplicasUnAvailable   string `json:"ReplicasUnAvailable"`
	ReplicasUpdated       string `json:"replicasUpdated"`
	CollisionCount        string `json:"collisionCount"`
	ReplicasReady         string `json:"replicasReady"`
}

func NewDeploySchemaDefWrapper() DeploySchemaDefWrapper {
	schema := NewDeploySchemaDef()
	wrapper := DeploySchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewDeploySchemaDef() DeploySchemaDef {
	pdsd := DeploySchemaDef{Name: "string", ClusterName: "string", Namespace: "string", ObjectUid: "string", CreationTimestamp: "date",
		DeletionTimestamp: "date", Labels: "string", Annotations: "string", MinReadySecs: "integer", ProgressDeadlineSecs: "integer",
		Replicas: "integer", RevisionHistoryLimits: "integer", Strategy: "string",
		MaxSurge: "string", MaxUnavailable: "string", ReplicasAvailable: "integer", ReplicasUnAvailable: "integer",
		ReplicasUpdated: "integer", CollisionCount: "integer", ReplicasReady: "integer"}
	return pdsd
}

type DeploySchema struct {
	Name                  string    `json:"name"`
	ClusterName           string    `json:"clusterName"`
	Namespace             string    `json:"namespace"`
	ObjectUid             string    `json:"object_uid"`
	CreationTimestamp     time.Time `json:"creationTimestamp"`
	DeletionTimestamp     time.Time `json:"deletionTimestamp"`
	Labels                string    `json:"labels"`
	Annotations           string    `json:"annotations"`
	MinReadySecs          int32     `json:"minReadySecs"`
	ProgressDeadlineSecs  int32     `json:"progressDeadlineSecs"`
	Replicas              int32     `json:"replicas"`
	RevisionHistoryLimits int32     `json:"revisionHistoryLimits"`
	Strategy              string    `json:"strategy"`
	MaxSurge              string    `json:"maxSurge"`
	MaxUnavailable        string    `json:"maxUnavailable"`
	ReplicasAvailable     int32     `json:"replicasAvailable"`
	ReplicasUnAvailable   int32     `json:"ReplicasUnAvailable"`
	ReplicasUpdated       int32     `json:"replicasUpdated"`
	CollisionCount        int32     `json:"collisionCount"`
	ReplicasReady         int32     `json:"replicasReady"`
}

type DeployObjList struct {
	Items []DeploySchema
}

func (ps *DeploySchema) Equals(obj *DeploySchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

func (ps *DeploySchema) GetDeployKey() string {
	return ps.Name
}

func NewDeployObjList() DeployObjList {
	return DeployObjList{}
}

func NewDeployObj() DeploySchema {
	return DeploySchema{MinReadySecs: 0, ProgressDeadlineSecs: 0, Replicas: 0, RevisionHistoryLimits: 0, ReplicasAvailable: 0,
		ReplicasUnAvailable: 0, ReplicasUpdated: 0, CollisionCount: 0, ReplicasReady: 0}
}

func (l DeployObjList) AddItem(obj DeploySchema) []DeploySchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l DeployObjList) Clear() []DeploySchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}

func CompareDeployObjects(newDeploy *appsv1.Deployment, oldDeploy *appsv1.Deployment) bool {
	return false
}
