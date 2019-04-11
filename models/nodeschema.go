package models

import (
	"fmt"
	"reflect"
	"strings"

	//	"time"
	"github.com/fatih/structs"
	"k8s.io/api/core/v1"
)

type NodeSchemaDefWrapper struct {
	Schema NodeSchemaDef `json:"schema"`
}

func (sd NodeSchemaDefWrapper) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

type NodeSchemaDef struct {
	NodeName        string `json:"nodeName"`
	ClusterName     string `json:"clusterName"`
	PodCIDR         string `json:"podCIDR"`
	Taints          string `json:"taints"`
	Phase           string `json:"phase"`
	Addresses       string `json:"addresses"`
	Labels          string `json:"labels"`
	Role            string `json:"role"`
	CpuUse          string `json:"cpuUse"`
	MemUse          string `json:"memUse"`
	CpuCapacity     string `json:"cpuCapacity"`
	MemCapacity     string `json:"memCapacity"`
	PodCapacity     string `json:"podCapacity"`
	CpuAllocations  string `json:"cpuAllocations"`
	MemAllocations  string `json:"memAllocations"`
	PodAllocations  string `json:"podAllocations"`
	KubeletPort     string `json:"kubeletPort"`
	OsArch          string `json:"osArch"`
	KubeletVersion  string `json:"kubeletVersion"`
	RuntimeVersion  string `json:"runtimeVersion"`
	MachineID       string `json:"machineID"`
	OsName          string `json:"osName"`
	AttachedVolumes string `json:"attachedVolumes"`
	VolumesInUse    string `json:"volumesInUse"`
	Ready           string `json:"ready"`
	OutOfDisk       string `json:"outOfDisk"`
	MemoryPressure  string `json:"memoryPressure"`
	DiskPressure    string `json:"diskPressure"`
}

func (sd NodeSchemaDef) Unwrap() *map[string]interface{} {
	objMap := structs.Map(sd)
	return &objMap
}

func NewNodeSchemaDefWrapper() NodeSchemaDefWrapper {
	schema := NewNodeSchemaDef()
	wrapper := NodeSchemaDefWrapper{Schema: schema}
	return wrapper
}

func NewNodeSchemaDef() NodeSchemaDef {
	pdsd := NodeSchemaDef{NodeName: "string", ClusterName: "string", PodCIDR: "string", Taints: "string", Phase: "string",
		Addresses: "string", Labels: "string", Role: "string", CpuUse: "float", MemUse: "float", CpuCapacity: "float", MemCapacity: "float", PodCapacity: "integer",
		CpuAllocations: "float", MemAllocations: "float", PodAllocations: "integer", KubeletPort: "integer", OsArch: "string",
		KubeletVersion: "string", RuntimeVersion: "string", MachineID: "string", OsName: "string", AttachedVolumes: "string", VolumesInUse: "string",
		Ready: "string", OutOfDisk: "string", MemoryPressure: "string", DiskPressure: "string"}
	return pdsd
}

type NodeSchema struct {
	NodeName        string `json:"nodeName"`
	ClusterName     string `json:"clusterName"`
	PodCIDR         string `json:"podCIDR"`
	Taints          string `json:"taints"`
	Phase           string `json:"phase"`
	Addresses       string `json:"addresses"`
	Labels          string `json:"labels"`
	Role            string `json:"role"`
	CpuUse          int64  `json:"cpuUse"`
	MemUse          int64  `json:"memUse"`
	CpuCapacity     int64  `json:"cpuCapacity"`
	MemCapacity     int64  `json:"memCapacity"`
	PodCapacity     int64  `json:"podCapacity"`
	CpuAllocations  int64  `json:"cpuAllocations"`
	MemAllocations  int64  `json:"memAllocations"`
	PodAllocations  int64  `json:"podAllocations"`
	KubeletPort     int32  `json:"kubeletPort"`
	OsArch          string `json:"osArch"`
	KubeletVersion  string `json:"kubeletVersion"`
	RuntimeVersion  string `json:"runtimeVersion"`
	MachineID       string `json:"machineID"`
	OsName          string `json:"osName"`
	AttachedVolumes string `json:"attachedVolumes"`
	VolumesInUse    string `json:"volumesInUse"`
	Ready           string `json:"ready"`
	OutOfDisk       string `json:"outOfDisk"`
	MemoryPressure  string `json:"memoryPressure"`
	DiskPressure    string `json:"diskPressure"`
	TaintsNumber    int    `json:"-"`
}

type NodeObjList struct {
	Items []NodeSchema
}

func (ps *NodeSchema) Equals(obj *NodeSchema) bool {
	return reflect.DeepEqual(*ps, *obj)
}

func (ps *NodeSchema) GetNodeKey() string {
	return ps.NodeName
}

func NewNodeObjList() NodeObjList {
	return NodeObjList{}
}

func NewNodeObj() NodeSchema {
	return NodeSchema{TaintsNumber: 0, MemCapacity: 0, MemAllocations: 0, CpuAllocations: 0, CpuCapacity: 0, PodAllocations: 0, PodCapacity: 0}
}

func (l NodeObjList) AddItem(obj NodeSchema) []NodeSchema {
	l.Items = append(l.Items, obj)
	return l.Items
}

func (l NodeObjList) Clear() []NodeSchema {
	l.Items = l.Items[:cap(l.Items)]
	return l.Items
}

func CompareNodeObjects(newNode *v1.Node, oldNode *v1.Node) bool {
	changed := []string{}
	if newNode.Name != oldNode.Name {
		changed = append(changed, "Name")
	}
	if newNode.Spec.PodCIDR != oldNode.Spec.PodCIDR {
		changed = append(changed, "PodCIDR")
	}

	if len(newNode.Spec.Taints) != len(oldNode.Spec.Taints) {
		changed = append(changed, "Taints")
	}

	if newNode.Status.Phase != oldNode.Status.Phase {
		changed = append(changed, "Phase")
	}

	if len(newNode.Status.Addresses) != len(oldNode.Status.Addresses) {
		changed = append(changed, "Addresses")
	}

	if len(newNode.Labels) != len(oldNode.Labels) {
		changed = append(changed, "Labels")
	}

	var ccpu, cmem, cpod int64
	for key, c := range newNode.Status.Capacity {
		if key == "memory" {
			cmem = c.Value()
		}
		if key == "cpu" {
			ccpu = c.Value()
		}
		if key == "pods" {
			cpod = c.Value()
		}
	}

	for key, c := range oldNode.Status.Capacity {
		if key == "memory" && cmem != c.Value() {
			changed = append(changed, "MemCapacity")
		}
		if key == "cpu" && ccpu != c.Value() {
			changed = append(changed, "CpuCapacity")
		}
		if key == "pods" && cpod != c.Value() {
			changed = append(changed, "PodCapacity")
		}
	}

	var acpu, amem, apod int64
	for k, a := range newNode.Status.Allocatable {
		if k == "memory" {
			amem = a.Value()
		}
		if k == "cpu" {
			acpu = a.Value()
		}
		if k == "pods" {
			apod = a.Value()
		}
	}

	for key, a := range oldNode.Status.Allocatable {
		if key == "memory" && amem != a.Value() {
			changed = append(changed, "MemAllocations")
		}
		if key == "cpu" && acpu != a.Value() {
			changed = append(changed, "CpuAllocations")
		}
		if key == "pods" && apod != a.Value() {
			changed = append(changed, "PodAllocations")
		}
	}

	if oldNode.Status.DaemonEndpoints.KubeletEndpoint.Port != oldNode.Status.DaemonEndpoints.KubeletEndpoint.Port {
		changed = append(changed, "KubeletPort")
	}

	if newNode.Status.NodeInfo.Architecture != oldNode.Status.NodeInfo.Architecture {
		changed = append(changed, "OsArch")

	}
	if newNode.Status.NodeInfo.KubeletVersion != oldNode.Status.NodeInfo.KubeletVersion {
		changed = append(changed, "KubeletVersion")
	}
	if newNode.Status.NodeInfo.ContainerRuntimeVersion != oldNode.Status.NodeInfo.ContainerRuntimeVersion {
		changed = append(changed, "RuntimeVersion")
	}
	if newNode.Status.NodeInfo.MachineID != oldNode.Status.NodeInfo.MachineID {
		changed = append(changed, "MachineID")
	}
	if newNode.Status.NodeInfo.OperatingSystem != oldNode.Status.NodeInfo.OperatingSystem {
		changed = append(changed, "OsName")
	}

	var ready, outofdisk, mempressure, diskpressure string
	for _, cond := range newNode.Status.Conditions {
		if cond.Type == "Ready" {
			status := strings.ToLower(string(cond.Status))
			ready = status
		}
		if cond.Type == "OutOfDisk" {
			status := strings.ToLower(string(cond.Status))
			outofdisk = status
		}
		if cond.Type == "MemoryPressure" {
			status := strings.ToLower(string(cond.Status))
			mempressure = status
		}
		if cond.Type == "DiskPressure" {
			status := strings.ToLower(string(cond.Status))
			diskpressure = status
		}
	}

	for _, cond := range oldNode.Status.Conditions {
		if cond.Type == "Ready" {
			status := strings.ToLower(string(cond.Status))
			if ready != status {
				changed = append(changed, "Ready")
			}
		}
		if cond.Type == "OutOfDisk" {
			status := strings.ToLower(string(cond.Status))
			if outofdisk != status {
				changed = append(changed, "OutOfDisk")
			}
		}
		if cond.Type == "MemoryPressure" {
			status := strings.ToLower(string(cond.Status))
			if mempressure != status {
				changed = append(changed, "MemoryPressure")
			}
		}
		if cond.Type == "DiskPressure" {
			status := strings.ToLower(string(cond.Status))
			if diskpressure != status {
				changed = append(changed, "DiskPressure")
			}
		}
	}

	if len(newNode.Status.VolumesAttached) != len(oldNode.Status.VolumesAttached) {
		changed = append(changed, "AttachedVolumes")
	}

	if len(newNode.Status.VolumesInUse) != len(oldNode.Status.VolumesInUse) {
		changed = append(changed, "VolumesInUse")
	}
	updated := len(changed) > 0
	if updated {
		fmt.Printf("Changed fields in Node: %s", strings.Join(changed, ";"))
	}
	return updated
}
