package models

import (
	"reflect"

	"time"

	"github.com/fatih/structs"
	appsv1 "k8s.io/api/apps/v1"
)

type RsSchemaDefWrapper struct {
	Schema RsSchemaDef `json:"schema"`
}

func (sd RsSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

type RsSchemaDef struct {
	Name                  string `json:"name"`
	ClusterName           string `json:"clusterName"`
	Namespace             string `json:"namespace"`
	ObjectUid             string `json:"object_uid"`
	CreationTimestamp     string `json:"creationTimestamp"`
	DeletionTimestamp     string `json:"deletionTimestamp"`
	MinReadySecs          string `json:"minReadySecs"`
	RsReplicas            string `json:"rsreplicas"`
	RsReplicasAvailable   string `json:"rsreplicasAvailable"`
	RsReplicasUnAvailable string `json:"rsReplicasUnAvailable"`
	RsReplicasLabeled     string `json:"rsReplicasLabeled"`
	RsReplicasReady       string `json:"replicasReady"`
}

func NewRsSchemaDefWrapper() RsSchemaDefWrapper {
	schema := NewRsSchemaDef()
	wrapper := RsSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewRsSchemaDef() RsSchemaDef {
	pdsd := RsSchemaDef{Name: "string", ClusterName: "string", Namespace: "string", ObjectUid: "string", CreationTimestamp: "date",
		DeletionTimestamp: "date", MinReadySecs: "integer",
		RsReplicas: "integer", RsReplicasAvailable: "integer", RsReplicasUnAvailable: "integer",
		RsReplicasLabeled: "integer", RsReplicasReady: "integer"}
	return pdsd
}

type RsSchema struct {
	Name                  string    `json:"name"`
	ClusterName           string    `json:"clusterName"`
	Namespace             string    `json:"namespace"`
	ObjectUid             string    `json:"object_uid"`
	CreationTimestamp     time.Time `json:"creationTimestamp"`
	DeletionTimestamp     time.Time `json:"deletionTimestamp"`
	MinReadySecs          int32     `json:"minReadySecs"`
	RsReplicas            int32     `json:"rsreplicas"`
	RsReplicasAvailable   int32     `json:"rsreplicasAvailable"`
	RsReplicasUnAvailable int32     `json:"rsReplicasUnAvailable"`
	RsReplicasLabeled     int32     `json:"rsReplicasLabeled"`
	RsReplicasReady       int32     `json:"replicasReady"`
}

type RsObjList struct {
	Items []RsSchema
}

func (ps *RsSchema) Equals(obj *RsSchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

func (ps *RsSchema) GetRsKey() string {
	return ps.Name
}

func NewRsObjList() RsObjList {
	return RsObjList{}
}

func NewRsObj() RsSchema {
	return RsSchema{MinReadySecs: 0, RsReplicas: 0, RsReplicasAvailable: 0,
		RsReplicasUnAvailable: 0, RsReplicasLabeled: 0, RsReplicasReady: 0}
}

func (l RsObjList) AddItem(obj RsSchema) []RsSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l RsObjList) Clear() []RsSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}

func CompareRsObjects(newRs *appsv1.ReplicaSet, oldRs *appsv1.ReplicaSet) bool {
	return false
}
