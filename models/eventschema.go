package models

import (
	"fmt"
	"reflect"
	"time"

	"github.com/fatih/structs"
)

type EventSchemaDefWrapper struct {
	Schema EventSchemaDef `json:"schema"`
}

func (sd EventSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

type EventSchemaDef struct {
	ObjectKind            string `json:"object_kind"`
	ObjectName            string `json:"object_name"`
	ClusterName           string `json:"clusterName"`
	ObjectNamespace       string `json:"object_namespace"`
	ObjectResourceVersion string `json:"object_resourceVersion"`
	ObjectUid             string `json:"object_uid"`
	LastTimestamp         string `json:"lastTimestamp"`
	Message               string `json:"message"`
	CreationTimestamp     string `json:"creationTimestamp"`
	DeletionTimestamp     string `json:"deletionTimestamp"`
	GenerateName          string `json:"generateName"`
	Generation            string `json:"generation"`
	Name                  string `json:"name"`
	Namespace             string `json:"namespace"`
	OwnerReferences       string `json:"ownerReferences"`
	ResourceVersion       string `json:"resourceVersion"`
	SelfLink              string `json:"selfLink"`
	Type                  string `json:"type"`
	Count                 string `json:"count"`
	SourceComponent       string `json:"source_component"`
	SourceHost            string `json:"source_host"`
	Reason                string `json:"reason"`
}

func NewEventSchemaDefWrapper() EventSchemaDefWrapper {
	schema := NewEventSchemaDef()
	wrapper := EventSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewEventSchemaDef() EventSchemaDef {
	pdsd := EventSchemaDef{ObjectKind: "string", ObjectName: "string", ClusterName: "string", ObjectNamespace: "string", ObjectResourceVersion: "string",
		ObjectUid: "string", LastTimestamp: "date", Message: "string", CreationTimestamp: "date", DeletionTimestamp: "date", GenerateName: "string", Generation: "integer",
		Name: "string", Namespace: "string", OwnerReferences: "string", ResourceVersion: "string",
		SelfLink: "string", Type: "string", Count: "integer", SourceComponent: "string", SourceHost: "string", Reason: "string"}
	return pdsd
}

type EventSchema struct {
	ObjectKind            string    `json:"object_kind"`
	ObjectName            string    `json:"object_name"`
	ClusterName           string    `json:"clusterName"`
	ObjectNamespace       string    `json:"object_namespace"`
	ObjectResourceVersion string    `json:"object_resourceVersion"`
	ObjectUid             string    `json:"object_uid"`
	LastTimestamp         time.Time `json:"lastTimestamp"`
	Message               string    `json:"message"`
	CreationTimestamp     time.Time `json:"creationTimestamp"`
	DeletionTimestamp     time.Time `json:"deletionTimestamp"`
	GenerateName          string    `json:"generateName"`
	Generation            int64     `json:"generation"`
	Name                  string    `json:"name"`
	Namespace             string    `json:"namespace"`
	OwnerReferences       string    `json:"ownerReferences"`
	ResourceVersion       string    `json:"resourceVersion"`
	SelfLink              string    `json:"selfLink"`
	Type                  string    `json:"type"`
	Count                 int32     `json:"count"`
	SourceComponent       string    `json:"source_component"`
	SourceHost            string    `json:"source_host"`
	Reason                string    `json:"reason"`
}

type EventObjList struct {
	Items []EventSchema
}

func (ps *EventSchema) Equals(obj *EventSchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

func NewEventObjList() EventObjList {
	return EventObjList{}
}

func NewEventObj() EventSchema {
	return EventSchema{}
}

func (p EventSchema) ToString() string {
	return fmt.Sprintf("ObjectKind: %s\n ObjectName: %s\n ClusterName: %s\n ObjectNamespace: %s\n ObjectResourceVersion: %s\n ObjectUid: %s\n LastTimestamp: %s\n Message: %s\n CreationTimestamp: %s\n DeletionTimestamp: %s\n  GenerateName: %s\n Generation: %s\n  Name: %s\n Namespace: %s\n OwnerReferences: %s\n ResourceVersion: %s\n SelfLink: %s\n Type: %s\n Count: %s\n SourceComponent: %s\n SourceHost: %s\n Reason: %s\n",
		p.ObjectKind, p.ObjectName, p.ClusterName, p.ObjectNamespace, p.ObjectResourceVersion, p.ObjectUid, p.LastTimestamp, p.Message, p.CreationTimestamp, p.DeletionTimestamp, p.GenerateName, p.Generation, p.Name, p.Namespace, p.OwnerReferences, p.ResourceVersion, p.SelfLink, p.Type, p.Count, p.SourceComponent, p.SourceHost, p.Reason)
}

func (l EventObjList) AddItem(obj EventSchema) []EventSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l EventObjList) Clear() []EventSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}
