package workers

import (
	"fmt"
	"strconv"
	"strings"

	m "github.com/sjeltuhin/clusterAgent/models"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	GET_JAVA_PID_CMD    string = "ps -o pid,comm,args | grep java | awk '{print$1,$2,$3}'"
	JDK_DIR             string = "$JAVA_HOME/lib/"
	ATTACHED_ANNOTATION string = "appd-attached"
)

type AgentInjector struct {
	ClientSet *kubernetes.Clientset
	K8sConfig *rest.Config
	Bag       *m.AppDBag
}

func NewAgentInjector(client *kubernetes.Clientset, config *rest.Config, bag *m.AppDBag) AgentInjector {
	return AgentInjector{ClientSet: client, K8sConfig: config, Bag: bag}
}

func (ai AgentInjector) EnsureInstrumentation(podObj *v1.Pod) error {
	//check if already instrumented
	for k, v := range podObj.Annotations {
		if k == ATTACHED_ANNOTATION && v == "true" {
			fmt.Printf("Pod %s already instrumented. Skipping...\n", podObj.Name)
			return nil
		}
	}

	var container *v1.Container = nil
	var appName, tierName string
	for k, v := range podObj.Labels {
		if k == ai.Bag.AppDAppLabel {
			appName = v
		}

		if k == ai.Bag.AppDTierLabel {
			tierName = v
		}
	}
	for _, c := range podObj.Spec.Containers {
		for _, vm := range c.VolumeMounts {
			if vm.Name == ai.Bag.AgentMountName {
				container = &c
				break
			}
		}
	}

	if appName != "" && tierName != "" && container != nil {
		var procName, args string
		var procID int = 0
		exec := NewExecutor(ai.ClientSet, ai.K8sConfig)
		code, output, err := exec.RunCommandInPod(podObj.Name, podObj.Namespace, container.Name, "", GET_JAVA_PID_CMD)
		if code == 0 {
			legit := true
			//split by line
			rows := strings.Split(output, "\n")
			for _, line := range rows {
				fields := strings.SplitN(line, " ", 3)

				if len(fields) > 1 {
					procName = fields[1]
				}
				if len(fields) > 2 {
					args = fields[2]
					if strings.Contains(args, "-DAppdynamics") {
						legit = false
						break
					}
				}
				pid, err := strconv.Atoi(fields[0])
				if err == nil {
					fmt.Printf("PID: %d, Name: %s, Args: %s\n", pid, procName, args)
					if strings.Contains(procName, "java") {
						procID = pid
					}
				}
			}
			if legit && procID > 0 {
				fmt.Printf("Instrumenting process %d\n", procID)
				ai.instrument(podObj, procID, appName, tierName, container.Name, &exec)
			}
		} else {
			fmt.Printf("Exec error code = %d. Output: %s, Error = %v\n", code, output, err)
		}
	}

	return nil

}

func (ai AgentInjector) instrument(podObj *v1.Pod, pid int, appName string, tierName string, containerName string, exec *Executor) error {
	cmd := fmt.Sprintf("java -Xbootclasspath/a:%s/tools.jar -jar %s/javaagent.jar %d appdynamics.controller.hostName=%s,appdynamics.controller.port=%d,appdynamics.controller.ssl.enabled=%t,appdynamics.agent.accountName=%s,appdynamics.agent.accountAccessKey=%s,appdynamics.agent.applicationName=%s,appdynamics.agent.tierName=%s,appdynamics.agent.reuse.nodeName=true,appdynamics.agent.reuse.nodeName.prefix=%s",
		ai.Bag.JDKMountPath, ai.Bag.AgentMountPath, pid, ai.Bag.ControllerUrl, ai.Bag.ControllerPort, ai.Bag.SSLEnabled, ai.Bag.Account, ai.Bag.AccessKey, appName, tierName, ai.Bag.NodeNamePrefix)

	code, output, err := exec.RunCommandInPod(podObj.Name, podObj.Namespace, containerName, "", cmd)
	if code == 0 {
		fmt.Printf("AppD agent attached. Output: %s. Error: %v\n", output, err)
		//annotate pod
		if podObj.Annotations == nil {
			podObj.Annotations = make(map[string]string)
		}
		podObj.Annotations[ATTACHED_ANNOTATION] = "true"
		_, updateErr := ai.ClientSet.Core().Pods(podObj.Namespace).Update(podObj)
		if updateErr != nil {

		}
	} else {
		fmt.Printf("Exec error code = %d. Output: %s, Error: %v\n", code, output, err)
	}
	return nil
}
