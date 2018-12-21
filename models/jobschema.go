package models

import (
	"fmt"
	"reflect"
	"time"
)

type JobSchemaDefWrapper struct {
	Schema JobSchemaDef `json:"schema"`
}

type JobSchemaDef struct {
	Name                  string `json:"name"`
	Namespace             string `json:"namespace"`
	ClusterName           string `json:"clusterName"`
	Labels                string `json:"labels"`
	Annotations           string `json:"annotations"`
	StartTime             string `json:"startTime"`
	EndTime               string `json:"endTime"`
	Active                string `json:"active"`
	Failed                string `json:"failed"`
	Success               string `json:"success"`
	ActiveDeadlineSeconds string `json:"activeDeadlineSeconds"`
	Completions           string `json:"completions"`
	BackoffLimit          string `json:"backoffLimit"`
	Parallelism           string `json:"parallelism"`
	Duration              string `json:"duration"`
}

func NewJobSchemaDefWrapper() JobSchemaDefWrapper {
	schema := NewJobSchemaDef()
	wrapper := JobSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewJobSchemaDef() JobSchemaDef {
	pdsd := JobSchemaDef{Name: "string", Namespace: "string", ClusterName: "string", Labels: "string", Annotations: "string", StartTime: "date", EndTime: "date",
		Active: "integer", Failed: "integer", Success: "integer", ActiveDeadlineSeconds: "integer", Completions: "integer", BackoffLimit: "integer", Parallelism: "integer", Duration: "float"}
	return pdsd
}

type JobSchema struct {
	Name                  string    `json:"name"`
	Namespace             string    `json:"namespace"`
	ClusterName           string    `json:"clusterName"`
	Labels                string    `json:"labels"`
	Annotations           string    `json:"annotations"`
	StartTime             time.Time `json:"startTime"`
	EndTime               time.Time `json:"endTime"`
	Active                int32     `json:"active"`
	Failed                int32     `json:"failed"`
	Success               int32     `json:"success"`
	ActiveDeadlineSeconds int64     `json:"activeDeadlineSeconds"`
	Completions           int32     `json:"completions"`
	BackoffLimit          int32     `json:"backoffLimit"`
	Parallelism           int32     `json:"parallelism"`
	Duration              float64   `json:"duration"`
}

type JobObjList struct {
	Items []JobSchema
}

func (ps *JobSchema) Equals(obj *JobSchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

func NewJobObjList() JobObjList {
	return JobObjList{}
}

func NewJobObj() JobSchema {
	return JobSchema{}
}

func (p JobSchema) ToString() string {
	return fmt.Sprintf("Name: %s\n Namespace: %s\n ClusterName: %s\n Labels: %s\n Annotations: %s\n StartTime: %s\n EndTime: %s\n Active: %d\n Failed: %d\n Success: %d\n ActiveDeadlineSeconds: %d\n Completions: %d\n BackoffLimit: %d\n Parallelism: %d\n Duration: %.2f\n",
		p.Name, p.Namespace, p.ClusterName, p.Labels, p.Annotations, p.StartTime, p.EndTime, p.Active, p.Failed, p.Success, p.ActiveDeadlineSeconds, p.Completions, p.BackoffLimit, p.Parallelism, p.Duration)
}

func (l JobObjList) AddItem(obj JobSchema) []JobSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l JobObjList) Clear() []JobSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}
