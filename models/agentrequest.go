package models

import (
	"fmt"
	"strings"

	"k8s.io/api/core/v1"
)

type TechnologyName string

const (
	REQUEST_SEPARATOR string         = ";"
	FIELD_SEPARATOR   string         = "_"
	Java              TechnologyName = "java"
	DotNet            TechnologyName = "dotnet"
	NodeJS            TechnologyName = "nodejs"
	ALL_CONTAINERS    string         = "all"
	FIRST_CONTAINER   string         = "all"
	VERSION_LATEST    string         = "latest"
)

type InstrumentationMethod string

const (
	None        InstrumentationMethod = "none"
	CopyAttach  InstrumentationMethod = "copyAttach"
	MountAttach InstrumentationMethod = "mountAttach"
	MountEnv    InstrumentationMethod = "mountEnv"
)

type AgentRequest struct {
	Namespaces    []string
	AppName       string
	TierName      string
	AppDAppLabel  string
	AppDTierLabel string
	Tech          TechnologyName
	ContainerName string //first (default), all, name
	Version       string
	MatchString   []string //string matched against deployment names and labels, supports regex
	Method        InstrumentationMethod
	BiQ           string //"sidecar" or reference to the remote analytics agent

}

type AgentRequestList struct {
	Items []AgentRequest
}

func (ar *AgentRequest) Clone() AgentRequest {
	clone := AgentRequest{}
	clone.Namespaces = ar.Namespaces
	clone.AppName = ar.AppName
	clone.AppDAppLabel = ar.AppDAppLabel
	clone.AppDTierLabel = ar.AppDTierLabel
	clone.Tech = ar.Tech
	clone.ContainerName = ar.ContainerName
	clone.Version = ar.Version
	clone.MatchString = []string{}
	for _, ms := range ar.MatchString {
		clone.MatchString = append(clone.MatchString, ms)
	}
	clone.Method = ar.Method
	clone.BiQ = ar.BiQ

	return clone
}

func NewAgentRequest(appdAgentLabel string, appName string, tierName string, biq string, bag *AppDBag) AgentRequest {
	agentRequest := AgentRequest{Method: bag.InstrumentationMethod, ContainerName: FIRST_CONTAINER}
	agentRequest.MatchString = []string{}
	if appdAgentLabel != "" {
		ar := strings.Split(appdAgentLabel, FIELD_SEPARATOR)
		if len(ar) > 0 {
			agentRequest.Tech = TechnologyName(ar[0])
			if len(ar) > 1 {
				agentRequest.ContainerName = ar[1]
				if len(ar) > 2 {
					agentRequest.Version = ar[2]
				}
			}

		}
	}
	agentRequest.AppName = appName
	agentRequest.TierName = tierName
	if agentRequest.Tech == "" {
		agentRequest.Tech = bag.DefaultInstrumentationTech
	}
	agentRequest.Version = VERSION_LATEST

	agentRequest.BiQ = biq

	if agentRequest.BiQ == "" && agentRequest.BiQ != string(NoBiq) {
		agentRequest.BiQ = bag.BiqService
	}

	return agentRequest
}

func NewAgentRequestListFromArray(ar []AgentRequest, bag *AppDBag, containers []v1.Container) *AgentRequestList {
	list := AgentRequestList{}
	index := 0
	add := false
	for _, r := range ar {
		fmt.Printf("AgentRequest Biq = %s\n", r.BiQ)
		if r.Method == "" {
			r.Method = bag.InstrumentationMethod
		}
		if r.Tech == "" {
			r.Tech = bag.DefaultInstrumentationTech
		}
		if r.AppDAppLabel == "" {
			r.AppDAppLabel = bag.AppDAppLabel
		}
		if r.AppDTierLabel == "" {
			r.AppDTierLabel = bag.AppDTierLabel
		}
		if len(ar) == 1 && r.ContainerName == ALL_CONTAINERS || (r.ContainerName == "" && bag.InstrumentContainer == ALL_CONTAINERS) {
			add = true
		}
		r.ContainerName = containers[index].Name
		if r.TierName == "" {
			r.TierName = r.ContainerName
		}
		if r.Version == "" {
			r.Version = VERSION_LATEST
		}
		if r.BiQ == "" {
			r.BiQ = bag.BiqService
		}

		list.Items = append(list.Items, r)
		index++
	}
	if add {
		clone := list.Items[0].Clone()
		for i := 1; i < len(containers); i++ {
			clone.ContainerName = containers[i].Name
			if clone.TierName == "" {
				clone.TierName = clone.ContainerName
			}
			list.Items = append(list.Items, clone)
		}
	}

	return &list
}

func NewAgentRequestList(appdAgentLabel string, appName string, tierName string, biq string, containers []v1.Container, bag *AppDBag) AgentRequestList {
	list := AgentRequestList{}

	if appdAgentLabel == "" {
		if bag.InstrumentContainer == FIRST_CONTAINER {
			def := getDefaultAgentRequest(appName, tierName, biq, bag)
			def.ContainerName = containers[0].Name
			if tierName == "" {
				def.TierName = def.ContainerName
			}
			list.Items = append(list.Items, def)
		} else {
			for _, c := range containers {
				def := getDefaultAgentRequest(appName, tierName, biq, bag)
				def.ContainerName = c.Name
				if tierName == "" {
					def.TierName = c.Name
				}
				list.Items = append(list.Items, def)
			}
		}
		return list
	}
	if strings.Contains(appdAgentLabel, REQUEST_SEPARATOR) {
		ar := strings.Split(appdAgentLabel, REQUEST_SEPARATOR)
		for _, a := range ar {
			ar := NewAgentRequest(a, appName, tierName, biq, bag)
			list.Items = append(list.Items, ar)
		}
	} else {
		ar := NewAgentRequest(appdAgentLabel, appName, tierName, biq, bag)
		if ar.ContainerName == "" {
			ar.ContainerName = bag.InstrumentContainer
		}
		if ar.ContainerName == FIRST_CONTAINER {
			ar.ContainerName = containers[0].Name
		}
		if tierName == "" {
			ar.TierName = ar.ContainerName
		}
		list.Items = append(list.Items, ar)
		if ar.ContainerName == ALL_CONTAINERS {
			for i := 1; i < len(containers); i++ {
				add := NewAgentRequest(appdAgentLabel, appName, tierName, biq, bag)
				if tierName == "" {
					add.TierName = add.ContainerName
				}
				add.ContainerName = containers[i].Name
			}
		}
	}

	return list
}

func getDefaultAgentRequest(appName string, tierName string, biq string, bag *AppDBag) AgentRequest {
	r := AgentRequest{ContainerName: FIRST_CONTAINER}
	r.AppName = appName
	r.TierName = tierName
	r.BiQ = biq
	r.Tech = bag.DefaultInstrumentationTech
	r.Method = bag.InstrumentationMethod
	r.MatchString = []string{}
	for _, ms := range bag.InstrumentMatchString {
		r.MatchString = append(r.MatchString, ms)
	}

	if r.BiQ == "" && r.BiQ != string(NoBiq) {
		r.BiQ = bag.BiqService
	}

	r.Version = VERSION_LATEST
	return r
}

func (al *AgentRequestList) ApplyInstrumentationMethod(m InstrumentationMethod) {

	for _, r := range al.Items {
		if r.Method == None {
			r.Method = m
		}
	}

}

func (al *AgentRequestList) GetRequest(cname string) *AgentRequest {
	var ar *AgentRequest = nil
	for _, r := range al.Items {
		if r.ContainerName == cname {
			ar = &r
			break
		}
	}
	if ar != nil {
		return ar
	}

	return al.GetFirstRequest()
}

func (al *AgentRequestList) GetFirstRequest() *AgentRequest {
	if len(al.Items) > 0 {
		return &(al.Items[0])
	}

	return nil
}

func (al *AgentRequestList) GetContainerNames() *[]string {
	var list []string

	for _, ar := range al.Items {
		if ar.ContainerName != "" {
			list = append(list, ar.ContainerName)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return &list
}

func (ar *AgentRequest) GetAgentImageName(bag *AppDBag) string {
	if ar.Version != "" && ar.Version != VERSION_LATEST {
		return ar.Version
	}
	if ar.Tech == Java {
		return bag.AppDJavaAttachImage
	}

	if ar.Tech == DotNet {
		return bag.AppDDotNetAttachImage
	}

	if ar.Tech == NodeJS {
		return bag.AppDNodeJSAttachImage
	}

	return ""

}

func (ar *AgentRequest) EnvRequired() bool {
	return ar.Method == MountEnv
}

func (al *AgentRequestList) EnvRequired() bool {
	for _, r := range al.Items {
		if r.EnvRequired() {
			return true
		}
	}
	return false
}

func (ar *AgentRequest) InitContainerRequired() bool {
	return ar.Method == MountAttach || ar.Method == MountEnv
}

func (al *AgentRequestList) InitContainerRequired() bool {
	for _, r := range al.Items {
		if r.InitContainerRequired() {
			return true
		}
	}
	return false
}

func (al *AgentRequestList) BiQRequested() bool {
	for _, r := range al.Items {
		if r.BiQRequested() {
			return true
		}
	}
	return false
}

func (ar *AgentRequest) BiQRequested() bool {

	return ar.BiQ != "" && ar.BiQ != string(NoBiq)
}

func (ar *AgentRequest) IsBiQRemote() bool {

	return ar.BiQRequested() && ar.BiQ != string(Sidecar)
}

func (al *AgentRequestList) GetBiQOption() string {
	//it is the same for
	for _, r := range al.Items {
		if r.BiQ != "" && r.BiQ != string(NoBiq) {
			return r.BiQ
		}
	}
	return ""
}

func (al *AgentRequestList) ToAnnotation() string {
	arrStrings := []string{}
	for _, r := range al.Items {
		arrStrings = append(arrStrings, r.ToAnnotation())
	}
	return strings.Join(arrStrings, REQUEST_SEPARATOR)
}

func (ar *AgentRequest) ToAnnotation() string {
	return fmt.Sprintf("%s_%s_%s_%s_%s_%s_%s", ar.Method, ar.Tech, ar.ContainerName, ar.AppName, ar.TierName, ar.BiQ, ar.Version)
}

func FromAnnotation(annotation string) *AgentRequestList {
	list := AgentRequestList{}
	if strings.Contains(annotation, ";") {
		ar := strings.Split(annotation, ";")
		for _, a := range ar {
			list.Items = append(list.Items, RequestFromAnnotation(a))
		}
	} else {
		list.Items = append(list.Items, RequestFromAnnotation(annotation))
	}
	return &list
}

func RequestFromAnnotation(annotation string) AgentRequest {
	r := AgentRequest{}
	arr := strings.Split(annotation, FIELD_SEPARATOR)
	if len(arr) > 0 {
		r.Method = InstrumentationMethod(arr[0])
	}
	if len(arr) > 1 {
		r.Method = InstrumentationMethod(arr[0])
		r.Tech = TechnologyName(arr[1])
	}
	if len(arr) > 2 {
		r.Method = InstrumentationMethod(arr[0])
		r.Tech = TechnologyName(arr[1])
		r.ContainerName = arr[2]
	}
	if len(arr) > 3 {
		r.Method = InstrumentationMethod(arr[0])
		r.Tech = TechnologyName(arr[1])
		r.ContainerName = arr[2]
		r.AppName = arr[3]
	}
	if len(arr) > 4 {
		r.Method = InstrumentationMethod(arr[0])
		r.Tech = TechnologyName(arr[1])
		r.ContainerName = arr[2]
		r.AppName = arr[3]
		r.TierName = arr[4]
	}
	if len(arr) > 5 {
		r.Method = InstrumentationMethod(arr[0])
		r.Tech = TechnologyName(arr[1])
		r.ContainerName = arr[2]
		r.AppName = arr[3]
		r.TierName = arr[4]
		r.BiQ = arr[5]
	}
	if len(arr) > 6 {
		r.Method = InstrumentationMethod(arr[0])
		r.Tech = TechnologyName(arr[1])
		r.ContainerName = arr[2]
		r.AppName = arr[3]
		r.TierName = arr[4]
		r.BiQ = arr[5]
		r.Version = arr[6]
	}
	return r
}

func (al *AgentRequestList) String() string {
	s := "Instrumentation requests:\n"
	for _, r := range al.Items {
		s = fmt.Sprintf("%s%s\n", s, r.String())
	}
	return s
}

func (ar *AgentRequest) String() string {
	return fmt.Sprintf("AppName: %s, TieName: %s, BiQ:%s, Tech: %s, Method: %s, Container: %s, Version: %s", ar.AppName, ar.TierName, ar.BiQ, ar.Tech, ar.Method, ar.ContainerName, ar.Version)
}
