package models

import (
	"fmt"
	"strings"
)

type TechnologyName string

const (
	Java   TechnologyName = "java"
	DotNet TechnologyName = "dotnet"
	NodeJS TechnologyName = "nodejs"
)

type InstrumentationMethod string

const (
	None     InstrumentationMethod = ""
	Copy     InstrumentationMethod = "copy"
	Mount    InstrumentationMethod = "mount"
	MountEnv InstrumentationMethod = "mountEnv"
)

type AgentRequest struct {
	AppDAppLabel  string
	AppDTierLabel string
	Tech          TechnologyName
	ContainerName string
	Version       string
	MatchString   string //string matched against deployment names and labels, supports regex
	Method        InstrumentationMethod
}

type AgentRequestList struct {
	Items    []AgentRequest
	AppName  string
	TierName string
}

func NewAgentRequest(appdAgentLabel string) AgentRequest {
	agentRequest := AgentRequest{Method: None}
	if appdAgentLabel != "" {
		ar := strings.Split(appdAgentLabel, "_")
		if len(ar) > 0 {
			agentRequest.Tech = TechnologyName(ar[0])
			if len(ar) > 1 {
				agentRequest.ContainerName = ar[1]
			}
			if len(ar) > 2 {
				agentRequest.Version = ar[2]
			}
		}
	}
	return agentRequest
}

func NewAgentRequestListFromArray(ar []AgentRequest) *AgentRequestList {
	list := AgentRequestList{}
	for _, r := range ar {
		list.Items = append(list.Items, r)
	}

	return &list
}

func NewAgentRequestListDetail(appName string, tierName string, method InstrumentationMethod, tech TechnologyName, contList *[]string) AgentRequestList {
	list := AgentRequestList{AppName: appName, TierName: tierName}

	if contList == nil {
		def := getDefaultAgentRequest()
		def.Tech = tech
		def.Method = method
		list.Items = append(list.Items, def)
		return list
	}

	for _, c := range *contList {
		r := AgentRequest{Method: None}
		r.ContainerName = c
		r.Method = method
		r.Tech = tech
	}

	return list
}

func NewAgentRequestList(appdAgentLabel string, appName string, tierName string) AgentRequestList {
	list := AgentRequestList{AppName: appName, TierName: tierName}

	if appdAgentLabel == "" {
		def := getDefaultAgentRequest()
		list.Items = append(list.Items, def)
		return list
	}
	if strings.Contains(appdAgentLabel, ";") {
		ar := strings.Split(appdAgentLabel, ";")
		for _, a := range ar {
			list.Items = append(list.Items, NewAgentRequest(a))
		}
	} else {
		list.Items = append(list.Items, NewAgentRequest(appdAgentLabel))
	}

	return list
}

func getDefaultAgentRequest() AgentRequest {
	return AgentRequest{Tech: Java, Method: None}
}

func (al *AgentRequestList) ApplyInstrumentationMethod(m InstrumentationMethod) {

	for _, r := range al.Items {
		if r.Method == None {
			r.Method = m
		}
	}

}

func (al *AgentRequestList) GetRequest(cname string) AgentRequest {
	var ar *AgentRequest = nil
	for _, r := range al.Items {
		if r.ContainerName == cname {
			ar = &r
			break
		}
	}
	if ar != nil {
		return *ar
	}

	return getDefaultAgentRequest()
}

func (al *AgentRequestList) GetFirstRequest() AgentRequest {
	if len(al.Items) > 0 {
		return al.Items[0]
	} else {
		return getDefaultAgentRequest()
	}
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

func (ar *AgentRequest) ToString() string {

	return fmt.Sprintf("Tech: %s, Container: %s, Version: %s", ar.Tech, ar.ContainerName, ar.Version)

}
