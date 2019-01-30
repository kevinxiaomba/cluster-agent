package utils

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
)

func IsPodRunnnig(podObj *v1.Pod) bool {
	return podObj != nil && podObj.Status.Phase == "Running" && podObj.DeletionTimestamp == nil
}

func GetDeployKey(deployObj *appsv1.Deployment) string {
	return fmt.Sprintf("%s_%s", deployObj.Namespace, deployObj.Name)
}

func GetPodKey(podObj *v1.Pod) string {
	return fmt.Sprintf("%s_%s", podObj.Namespace, podObj.Name)
}

func GetServiceKey(svc *v1.Service) string {
	return fmt.Sprintf("%s_%s", svc.Namespace, svc.Name)
}

func GetEndpointKey(ep *v1.Endpoints) string {
	return fmt.Sprintf("%s_%s", ep.Namespace, ep.Name)
}

func StringInSlice(s string, list []string) bool {
	return GetIndex(s, list) > -1
}

func GetIndex(s string, list []string) int {
	index := -1
	for i, b := range list {
		if b == s {
			index = i
			break
		}
	}
	return index
}

func RemoveFromSlice(s string, list []string) []string {
	i := GetIndex(s, list)
	if i < 0 {
		return list
	}
	list[i] = list[len(list)-1]

	return list[:len(list)-1]
}
