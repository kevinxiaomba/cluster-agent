package models

import (
	//	"fmt"
	//	"strings"

	"k8s.io/api/core/v1"

	"github.com/fatih/structs"
)

type NsSchemaDefWrapper struct {
	Schema NsSchemaDef `json:"schema"`
}

type NsSchemaDef struct {
	ClusterName string `json:"clusterName"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Quotas      string `json:"quotas"`
}

func (sd NsSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

func NewNsSchemaDefWrapper() NsSchemaDefWrapper {
	schema := NewNsSchemaDef()
	wrapper := NsSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewNsSchemaDef() NsSchemaDef {
	pdsd := NsSchemaDef{ClusterName: "string", Status: "string", Name: "string",
		Quotas: "integer"}
	return pdsd
}

type NsSchema struct {
	ClusterName string `json:"clusterName"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Quotas      int    `json:"quotas"`
}

func NewNsSchema(ns *v1.Namespace, bag *AppDBag) NsSchema {

	n := NsSchema{Name: ns.Name, ClusterName: ns.ClusterName, Status: string(ns.Status.Phase), Quotas: -1}
	if n.ClusterName == "" {
		n.ClusterName = bag.AppName
	}
	return n
}
