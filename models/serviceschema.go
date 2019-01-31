package models

import (
	"fmt"
	"strconv"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type ServiceSchema struct {
	Name                 string
	Namespace            string
	Selector             map[string]string
	Ports                []ServicePort
	Routes               []Ingress
	Endpoints            map[int32]ServiceEndpoint
	ExternalName         string
	ExternalSvcValid     bool
	HasNodePort          bool
	HasIngress           bool
	HasExternalService   bool
	IsAccessible         bool
	ReferencedExternally bool
}

type Ingress struct {
	Hostname string
	IP       string
}

type ServicePort struct {
	Name       string
	Port       int32
	TargetPort int32
	IsNodePort bool
	Protocol   string
}

type ServiceEndpoint struct {
	Name        string
	Port        int32
	EPAddresses []EPAddress
}

type EPAddress struct {
	Hostname string
	IP       string
	Nodename *string
	Ready    bool
}

func (svc *ServiceSchema) GetEndPointsStats() (int, map[string]int, map[string]int) {
	total := 0
	ready := make(map[string]int)
	notready := make(map[string]int)
	for _, ep := range svc.Endpoints {
		key := strconv.Itoa(int(ep.Port))
		r := 0
		nr := 0
		for _, epa := range ep.EPAddresses {
			total++
			if epa.Ready {
				r++
				ready[key] = r
			} else {
				nr++
				notready[key] = r
			}
		}
	}
	if svc.ExternalName != "" {
		total++
		if svc.ExternalSvcValid {
			ready[svc.ExternalName] = 1
		} else {
			notready[svc.ExternalName] = 1
		}
	}
	return total, ready, notready
}

func NewServiceEndpoint(name string, portNumber int32) ServiceEndpoint {
	return ServiceEndpoint{Name: name, Port: portNumber, EPAddresses: []EPAddress{}}
}

func NewServiceEndpointAddress(IP string, hostname string, nodename *string) EPAddress {
	return EPAddress{IP: IP, Hostname: hostname, Nodename: nodename}
}

func NewServiceSchema(svc *v1.Service) *ServiceSchema {
	schema := ServiceSchema{Ports: []ServicePort{}, Routes: []Ingress{}, Selector: make(map[string]string), Endpoints: make(map[int32]ServiceEndpoint)}
	schema.Name = svc.Name
	schema.Namespace = svc.Namespace
	schema.Selector = svc.Spec.Selector
	schema.ExternalName = svc.Spec.ExternalName
	schema.HasExternalService = schema.ExternalName != ""

	for _, sp := range svc.Spec.Ports {
		port := ServicePort{}
		port.Name = sp.Name
		portNumber := sp.Port
		port.TargetPort = sp.TargetPort.IntVal
		if sp.NodePort > 0 {
			portNumber = sp.NodePort
			port.IsNodePort = true
			schema.HasNodePort = true
			fmt.Printf("Service exposes nodePort %d\n", portNumber)
		}
		port.Port = portNumber
		port.Protocol = string(sp.Protocol)
		schema.Ports = append(schema.Ports, port)
	}

	for _, route := range svc.Status.LoadBalancer.Ingress {
		ingress := Ingress{Hostname: route.Hostname, IP: route.IP}
		schema.HasIngress = true
		schema.Routes = append(schema.Routes, ingress)
		fmt.Printf("Ingress %s %s found for service %s \n", ingress.Hostname, ingress.IP, schema.Name)
	}

	schema.IsAccessible = schema.HasIngress || schema.HasNodePort || schema.HasExternalService || schema.ReferencedExternally
	if !schema.IsAccessible {
		fmt.Printf("Service %s does not have external connectivity\n", schema.Name)
	}
	return &schema
}

func (svcSchema *ServiceSchema) ExternallyReferenced(referenced bool) {
	svcSchema.ReferencedExternally = referenced
	if referenced {
		svcSchema.IsAccessible = true
	}
}

func (svcSchema *ServiceSchema) MatchesPod(podObject *v1.Pod) bool {
	podLabels := labels.Set(podObject.Labels)
	svcSelector := labels.SelectorFromSet(svcSchema.Selector)
	return svcSchema.Namespace == podObject.Namespace && !svcSelector.Empty() && svcSelector.Matches(podLabels)
}

func (svcSchema *ServiceSchema) IsPortAvailable(cp *ContainerPort, podSchema *PodSchema) (bool, bool) {
	if podSchema.Namespace != "devops" {
		return false, false
	}

	var mapped, ready bool = false, false
	for _, sp := range svcSchema.Ports {
		if sp.TargetPort == cp.PortNumber {
			fmt.Printf("Port %d found. Name %s. Port Mapped\n", sp.TargetPort, sp.Name)
			mapped = true
			//check readiness
			for _, ep := range podSchema.Endpoints {
				if ep.Name != svcSchema.Name {
					continue
				}
				var endpointObj ServiceEndpoint
				var ok bool = false

				for _, eps := range ep.Subsets {
					portFound := false
					for _, epsPort := range eps.Ports {
						if epsPort.Port == cp.PortNumber {
							fmt.Printf("Endpoint Port %d found.\n", epsPort.Port)
							portFound = true
							endpointObj, ok = svcSchema.Endpoints[epsPort.Port]
							if !ok {
								endpointObj = NewServiceEndpoint(ep.Name, epsPort.Port)
							} else {
								endpointObj.Port = epsPort.Port
							}

							break
						}
					}
					if portFound {
						//checking health
						for _, epsAddr := range eps.Addresses {
							fmt.Printf("Checking IP %s against %s\n", epsAddr.IP, podSchema.PodIP)
							if epsAddr.IP == podSchema.PodIP {
								addr := NewServiceEndpointAddress(epsAddr.IP, epsAddr.Hostname, epsAddr.NodeName)
								addr.Ready = true
								endpointObj.EPAddresses = append(endpointObj.EPAddresses, addr)
								fmt.Printf("Healthy address  %s. Port %d ready\n", epsAddr.IP, cp.PortNumber)
								ready = true
								break
							}
						}
						for _, epsNRAdrr := range eps.NotReadyAddresses {
							fmt.Printf("Checking notready IP %s against %s\n", epsNRAdrr.IP, podSchema.PodIP)
							if epsNRAdrr.IP == podSchema.PodIP {
								addr := NewServiceEndpointAddress(epsNRAdrr.IP, epsNRAdrr.Hostname, epsNRAdrr.NodeName)
								addr.Ready = false
								endpointObj.EPAddresses = append(endpointObj.EPAddresses, addr)
								fmt.Printf("NotReady address  %s. Port %d not available\n", epsNRAdrr.IP, cp.PortNumber)
								ready = false
								break
							}
						}
					}
				}
				svcSchema.Endpoints[endpointObj.Port] = endpointObj
			}
		}
	}

	return mapped, ready
}
