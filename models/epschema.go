package models

import (
	"fmt"
	"strings"

	"k8s.io/api/core/v1"

	"github.com/fatih/structs"
)

type EpSchemaDefWrapper struct {
	Schema EpSchemaDef `json:"schema"`
}

type EpSchemaDef struct {
	ClusterName string `json:"clusterName"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Ports       string `json:"ports"`
	ReadyIPs    string `json:"readyIPs"`
	NotReadyIPs string `json:"notReadyIPs"`
	Pods        string `json:"pods"`
	PodCount    string `json:"podCount"`
	IsOrphan    string `json:"isOrphan"`
}

func (sd EpSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

func NewEpSchemaDefWrapper() EpSchemaDefWrapper {
	schema := NewEpSchemaDef()
	wrapper := EpSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewEpSchemaDef() EpSchemaDef {
	pdsd := EpSchemaDef{ClusterName: "string", Namespace: "string", Name: "string",
		Ports: "string", ReadyIPs: "string", NotReadyIPs: "string", Pods: "string", PodCount: "integer", IsOrphan: "boolean"}
	return pdsd
}

type EpSchema struct {
	ClusterName string `json:"clusterName"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Ports       string `json:"ports"`
	ReadyIPs    string `json:"readyIPs"`
	NotReadyIPs string `json:"notReadyIPs"`
	Pods        string `json:"pods"`
	PodCount    int    `json:"podCount"`
	IsOrphan    bool   `json:"isOrphan"`
}

func NewEpSchema(ep *v1.Endpoints) EpSchema {
	epSchema := EpSchema{}
	epSchema.Namespace = ep.Namespace

	var sb strings.Builder
	var sbReady strings.Builder
	var sbNotReady strings.Builder
	readyCount := 0
	notReadyCount := 0
	for _, eps := range ep.Subsets {
		for _, epsPort := range eps.Ports {
			fmt.Fprintf(&sb, "%d;", epsPort.Port)
		}

		for _, epsAddr := range eps.Addresses {
			fmt.Fprintf(&sbReady, "%s;", epsAddr.IP)
			readyCount++
		}
		for _, epsNRAdrr := range eps.NotReadyAddresses {
			fmt.Fprintf(&sbNotReady, "%s;", epsNRAdrr.IP)
			notReadyCount++
		}
	}
	epSchema.Ports = sb.String()
	epSchema.ReadyIPs = sbReady.String()
	epSchema.NotReadyIPs = sbNotReady.String()
	epSchema.IsOrphan = (notReadyCount == 0 && readyCount == 0)

	return epSchema
}

func (ep *EpSchema) MatchPod(podSchema *PodSchema) {
	for _, e := range podSchema.Endpoints {
		if e.Name == ep.Name && e.Namespace == ep.Namespace {
			ep.Pods = fmt.Sprintf("%s%s;", ep.Pods, podSchema.Name)
			ep.PodCount++
		}
	}
}
