package models

import (
	"fmt"
	"reflect"
	"time"
)

type LogSchemaDefWrapper struct {
	Schema LogSchemaDef `json:"schema"`
}

type LogSchemaDef struct {
	ClusterName    string `json:"clusterName"`
	Namespace      string `json:"namespace"`
	PodOwner       string `json:"podOwner"`
	PodName        string `json:"pod"`
	ContainerName  string `json:"container"`
	Message        string `json:"message"`
	BatchTimestamp string `json:"batchTimestamp"`
	Timestamp      string `json:"timestamp"`
}

func NewLogSchemaDefWrapper() LogSchemaDefWrapper {
	schema := NewLogSchemaDef()
	wrapper := LogSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewLogSchemaDef() LogSchemaDef {
	pdsd := LogSchemaDef{ClusterName: "string", Namespace: "string", PodOwner: "string", PodName: "string", ContainerName: "string", Message: "string",
		BatchTimestamp: "integer", Timestamp: "string"}
	return pdsd
}

type LogSchema struct {
	ClusterName    string     `json:"clusterName"`
	Namespace      string     `json:"namespace"`
	PodOwner       string     `json:"podOwner"`
	PodName        string     `json:"pod"`
	ContainerName  string     `json:"container"`
	Message        string     `json:"message"`
	BatchTimestamp int64      `json:"batchTimestamp"`
	Timestamp      *time.Time `json:"timestamp"`
}

type LogObjList struct {
	Items []LogSchema
}

func (ps *LogSchema) Equals(obj *LogSchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

func NewLogObjList() LogObjList {
	return LogObjList{}
}

func NewLogObj() LogSchema {
	return LogSchema{}
}

func (p LogSchema) ToString() string {
	return fmt.Sprintf("ClusterName: %s\n Namespace: %s\n PodOwner: %s\n PodName: %s\n ContainerName: %s\n Message: %s\n BatchTimestamp: %s\n Timestamp: %s\n ",
		p.ClusterName, p.Namespace, p.PodOwner, p.PodName, p.ContainerName, p.Message, p.Message, p.BatchTimestamp, p.Timestamp)
}

func (l LogObjList) AddItem(obj LogSchema) []LogSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l LogObjList) Clear() []LogSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}
