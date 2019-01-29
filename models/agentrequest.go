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
	Copy  InstrumentationMethod = "copy"
	Mount InstrumentationMethod = "mount"
)

type AgentRequest struct {
	Tech          TechnologyName
	ContainerName string
	Version       string
}

type AgentRequestList struct {
	Items    []AgentRequest
	AppName  string
	TierName string
}

func NewAgentRequest(appdAgentLabel string) AgentRequest {
	agentRequest := AgentRequest{}
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
	return AgentRequest{Tech: Java}
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
