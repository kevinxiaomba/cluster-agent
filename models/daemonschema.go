package models

import (
	"reflect"

	"time"

	appsv1 "k8s.io/api/apps/v1"
)

type DaemonSchemaDefWrapper struct {
	Schema DaemonSchemaDef `json:"schema"`
}

type DaemonSchemaDef struct {
	Name                   string `json:"name"`
	ClusterName            string `json:"clusterName"`
	Namespace              string `json:"namespace"`
	ObjectUid              string `json:"object_uid"`
	CreationTimestamp      string `json:"creationTimestamp"`
	DeletionTimestamp      string `json:"deletionTimestamp"`
	MinReadySecs           string `json:"minReadySecs"`
	RevisionHistoryLimits  string `json:"revisionHistoryLimits"`
	ReplicasAvailable      string `json:"replicasAvailable"`
	ReplicasUnAvailable    string `json:"ReplicasUnAvailable"`
	CollisionCount         string `json:"collisionCount"`
	ReplicasReady          string `json:"replicasReady"`
	NumberScheduled        string `json:"numberScheduled"`
	DesiredNumber          string `json:"desiredNumber"`
	MissScheduled          string `json:"missScheduled"`
	UpdatedNumberScheduled string `json:"updatedNumberScheduled"`
}

func NewDaemonSchemaDefWrapper() DaemonSchemaDefWrapper {
	schema := NewDaemonSchemaDef()
	wrapper := DaemonSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewDaemonSchemaDef() DaemonSchemaDef {
	pdsd := DaemonSchemaDef{Name: "string", ClusterName: "string", Namespace: "string", ObjectUid: "string", CreationTimestamp: "date",
		DeletionTimestamp: "date", MinReadySecs: "integer", RevisionHistoryLimits: "integer",
		ReplicasAvailable: "integer", ReplicasUnAvailable: "integer",
		CollisionCount: "integer", ReplicasReady: "integer", NumberScheduled: "integer", DesiredNumber: "integer",
		MissScheduled: "integer", UpdatedNumberScheduled: "integer"}
	return pdsd
}

type DaemonSchema struct {
	Name                   string    `json:"name"`
	ClusterName            string    `json:"clusterName"`
	Namespace              string    `json:"namespace"`
	ObjectUid              string    `json:"object_uid"`
	CreationTimestamp      time.Time `json:"creationTimestamp"`
	DeletionTimestamp      time.Time `json:"deletionTimestamp"`
	MinReadySecs           int32     `json:"minReadySecs"`
	RevisionHistoryLimits  int32     `json:"revisionHistoryLimits"`
	ReplicasAvailable      int32     `json:"replicasAvailable"`
	ReplicasUnAvailable    int32     `json:"ReplicasUnAvailable"`
	CollisionCount         int32     `json:"collisionCount"`
	ReplicasReady          int32     `json:"replicasReady"`
	NumberScheduled        int32     `json:"numberScheduled"`
	DesiredNumber          int32     `json:"desiredNumber"`
	MissScheduled          int32     `json:"missScheduled"`
	UpdatedNumberScheduled int32     `json:"updatedNumberScheduled"`
}

type DaemonObjList struct {
	Items []DaemonSchema
}

func (ps *DaemonSchema) Equals(obj *DaemonSchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

func (ps *DaemonSchema) GetDaemonKey() string {
	return ps.Name
}

func NewDaemonObjList() DaemonObjList {
	return DaemonObjList{}
}

func NewDaemonObj() DaemonSchema {
	return DaemonSchema{MinReadySecs: 0, RevisionHistoryLimits: 0, ReplicasAvailable: 0,
		ReplicasUnAvailable: 0, UpdatedNumberScheduled: 0, CollisionCount: 0, ReplicasReady: 0, NumberScheduled: 0,
		DesiredNumber: 0, MissScheduled: 0}
}

func (l DaemonObjList) AddItem(obj DaemonSchema) []DaemonSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l DaemonObjList) Clear() []DaemonSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}

func CompareDaemonObjects(newDaemon *appsv1.DaemonSet, oldDaemon *appsv1.DaemonSet) bool {
	return false
}
