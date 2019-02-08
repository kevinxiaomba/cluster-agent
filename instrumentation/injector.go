package instrumentation

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	app "github.com/sjeltuhin/clusterAgent/appd"

	m "github.com/sjeltuhin/clusterAgent/models"
	"github.com/sjeltuhin/clusterAgent/utils"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	GET_JAVA_PID_CMD             string = "ps -o pid,comm,args | grep java | awk '{print$1,$2,$3}'"
	JDK_DIR                      string = "$JAVA_HOME/lib/"
	APPD_DIR                     string = "/opt/appd/"
	ATTACHED_ANNOTATION          string = "appd-attached"
	DEPLOY_ANNOTATION            string = "appd-deploy-updated"
	DEPLOY_BIQ_ANNOTATION        string = "appd-deploy-biq-updated"
	APPD_APPID                   string = "appd-appid"
	APPD_TIERID                  string = "appd-tierid"
	APPD_NODEID                  string = "appd-nodeid"
	APPD_NODENAME                string = "appd-nodename"
	MAX_INSTRUMENTATION_ATTEMPTS int    = 1
	ANNOTATION_UPDATE_ERROR      string = "ANNOTATION-UPDATE-FAILURE"
)

type AgentInjector struct {
	ClientSet      *kubernetes.Clientset
	K8sConfig      *rest.Config
	Bag            *m.AppDBag
	AppdController *app.ControllerClient
}

func NewAgentInjector(client *kubernetes.Clientset, config *rest.Config, bag *m.AppDBag, appdController *app.ControllerClient) AgentInjector {
	return AgentInjector{ClientSet: client, K8sConfig: config, Bag: bag, AppdController: appdController}
}

func AnalyticsAgentExists(podSpec *v1.PodSpec, bag *m.AppDBag) bool {
	exists := false

	for _, c := range podSpec.Containers {
		if c.Name == bag.AnalyticsAgentContainerName {
			exists = true
			break
		}
	}
	return exists
}

func AgentInitExists(podSpec *v1.PodSpec, bag *m.AppDBag) bool {
	exists := false

	for _, c := range podSpec.InitContainers {
		if c.Name == bag.AppDInitContainerName {
			exists = true
			break
		}
	}
	return exists
}

func IsPodInstrumented(podObj *v1.Pod) bool {
	for k, v := range podObj.Annotations {
		if k == ATTACHED_ANNOTATION && v != "" {
			fmt.Printf("Pod %s already instrumented. Skipping...\n", podObj.Name)
			return true
		}
	}
	return false
}

func (ai AgentInjector) EnsureInstrumentation(statusChanel chan m.AttachStatus, podObj *v1.Pod, podSchema *m.PodSchema) error {
	//check if already instrumented
	if IsPodInstrumented(podObj) {
		statusChanel <- ai.buildAttachStatus(podObj, nil, true)
		return nil
	}

	var appName, tierName, appAgent string
	var biQDeploymentOption m.BiQDeploymentOption
	for k, v := range podObj.Labels {
		if k == ai.Bag.AppDAppLabel {
			appName = v
		}

		if k == ai.Bag.AppDTierLabel {
			tierName = v
		}

		if k == ai.Bag.AgentLabel {
			appAgent = v
		}

		if k == ai.Bag.AppDAnalyticsLabel {
			biQDeploymentOption = m.BiQDeploymentOption(v)
		}
	}

	if appName != "" {
		fmt.Println("Instrumentation requested. Checking for necessary components...")

		//when mounting, init container is needed
		if ai.Bag.InstrumentationMethod == m.Mount && !AgentInitExists(&podObj.Spec, ai.Bag) {
			fmt.Printf("Deployment of pod %s needs to be updated with init container. Skipping instrumentation until the deployment update is complete...\n", podObj.Name)
			statusChanel <- ai.buildAttachStatus(podObj, nil, true)
			return nil
		}
		//if biq requested, make sure side car is in the spec
		if biQDeploymentOption == m.Sidecar && !AnalyticsAgentExists(&podObj.Spec, ai.Bag) {
			fmt.Printf("Deployment of pod %s needs to be updated. Skipping instrumentation until the deployment update is complete...\n", podObj.Name)
			statusChanel <- ai.buildAttachStatus(podObj, nil, true)
			return nil
		}

		if tierName == "" {
			tierName = podSchema.Owner
		}

		agentRequests := m.NewAgentRequestList(appAgent, appName, tierName)

		if agentRequests.GetFirstRequest().Tech == m.DotNet {
			fmt.Println("DotNet workloads do not require special attachment. Annotating and aborting...")
			podObj.Annotations[ATTACHED_ANNOTATION] = time.Now().String()
			_, updateErr := ai.ClientSet.Core().Pods(podObj.Namespace).Update(podObj)
			if updateErr != nil {
				statusChanel <- ai.buildAttachStatus(podObj, fmt.Errorf("%s, Error: %v\n", ANNOTATION_UPDATE_ERROR, updateErr), false)
			} else {
				statusChanel <- ai.buildAttachStatus(podObj, nil, false)
			}
			return nil
		}

		containersToInstrument := agentRequests.GetContainerNames()

		found := false
		//if containers for instrumentation are defined, use them, otherwise pick the first
		for _, c := range podObj.Spec.Containers {
			if containersToInstrument == nil {
				found = true
				//instrument first container
				fmt.Printf("No specific requests. Instrumenting first container %s\n", c.Name)
				err := ai.instrumentContainer(appName, tierName, &c, podObj, biQDeploymentOption)
				statusChanel <- ai.buildAttachStatus(podObj, err, false)
				break
			} else {
				if utils.StringInSlice(c.Name, *containersToInstrument) {
					found = true
					fmt.Printf("Container %s requested. Instrumenting...\n", c.Name)
					err := ai.instrumentContainer(appName, tierName, &c, podObj, biQDeploymentOption)
					statusChanel <- ai.buildAttachStatus(podObj, err, false)
				}
			}
		}
		if !found {
			statusChanel <- ai.buildAttachStatus(podObj, nil, true)
		}
	} else {
		fmt.Println("Instrumentation not requested. Aborting...")
		statusChanel <- ai.buildAttachStatus(podObj, nil, true)
	}

	return nil

}

func (ai AgentInjector) buildAttachStatus(podObj *v1.Pod, err error, skip bool) m.AttachStatus {
	if err == nil {
		return m.AttachStatus{Key: utils.GetPodKey(podObj), Success: !skip}
	}
	message := err.Error()
	st := m.AttachStatus{Key: utils.GetPodKey(podObj), Count: 1, LastAttempt: time.Now(), LastMessage: message}
	if strings.Contains(message, ANNOTATION_UPDATE_ERROR) {
		st.Success = true
	}
	return st
}

func (ai AgentInjector) instrumentContainer(appName string, tierName string, container *v1.Container, podObj *v1.Pod, biQDeploymentOption m.BiQDeploymentOption) error {
	var procName, args string
	var procID int = 0
	exec := NewExecutor(ai.ClientSet, ai.K8sConfig)

	if ai.Bag.InstrumentationMethod == m.Copy {
		//copy files
		err := ai.copyArtifactsSync(&exec, podObj, container.Name)
		if err != nil {
			return fmt.Errorf("Unable to instrument. Failed to copy necessary artifacts into the pod. %v", err)
		}
		fmt.Println("Artifacts copied. Starting instrumentation...")
	} else {
		fmt.Println("Artifacts mounted. Starting instrumentation...")
	}

	//run attach
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
			return ai.instrument(podObj, procID, appName, tierName, container.Name, &exec, biQDeploymentOption)
		} else {
			return fmt.Errorf("Enable to determine process to instrument")
		}
	} else {
		return fmt.Errorf("Exec error code = %d. Output: %s, Error = %v\n", code, output, err)
	}
	return nil
}

func (ai AgentInjector) copyArtifactsSync(exec *Executor, podObj *v1.Pod, containerName string) error {
	bth := ai.AppdController.StartBT("CopyArtifacts")
	err := ai.copyFileSync(exec, "assets/tools.jar", ai.Bag.AgentMountPath, podObj, containerName)
	if err == nil {
		err = ai.copyFileSync(exec, "assets/AppServerAgent/", ai.Bag.AgentMountPath, podObj, containerName)
	}
	ai.AppdController.StopBT(bth)
	return err
}

func (ai AgentInjector) copyArtifacts(exec *Executor, podObj *v1.Pod, containerName string) bool {
	okJDKChan := make(chan bool)
	okAppDChan := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(1)
	//JDK tools
	go ai.copyFile(okJDKChan, &wg, exec, "assets/tools.jar", ai.Bag.AgentMountPath, podObj, containerName)

	wg.Add(1)
	//AppD Agent
	go ai.copyFile(okAppDChan, &wg, exec, "assets/AppServerAgent/", ai.Bag.AgentMountPath, podObj, containerName)

	okJDK := <-okJDKChan
	okAppD := <-okAppDChan

	wg.Wait()

	if !okJDK || !okAppD {
		return false
	}
	return true
}

func (ai AgentInjector) copyFileSync(exec *Executor, fileName, dir string, podObj *v1.Pod, containerName string) error {
	filePath, errFile := filepath.Abs(fileName)
	if errFile != nil {
		fmt.Printf("Cannot find %s path. %v\n", filePath, errFile)
		return errFile
	}
	fmt.Printf("Copying file %s\n", filePath)
	_, _, copyErr := exec.CopyFilesToPod(podObj, containerName, filePath, dir)
	if copyErr != nil {
		fmt.Printf("Copy failed. Aborting instrumentation. %v\n", copyErr)
		return copyErr
	}
	return nil
}

func (ai AgentInjector) copyFile(ok chan bool, wg *sync.WaitGroup, exec *Executor, fileName, dir string, podObj *v1.Pod, containerName string) {
	defer wg.Done()
	filePath, errFile := filepath.Abs(fileName)
	if errFile != nil {
		fmt.Printf("Cannot find %s path. %v\n", filePath, errFile)
		ok <- false
	}
	fmt.Printf("Copying file %s\n", filePath)
	_, _, copyErr := exec.CopyFilesToPod(podObj, containerName, filePath, dir)
	if copyErr != nil {
		fmt.Printf("Copy failed. Aborting instrumentation. %v\n", copyErr)
		ok <- false
	}
	ok <- true
}

func (ai AgentInjector) instrument(podObj *v1.Pod, pid int, appName string, tierName string, containerName string, exec *Executor, biQDeploymentOption m.BiQDeploymentOption) error {
	jdkPath := ai.Bag.AgentMountPath
	jarPath := ai.Bag.AgentMountPath
	if ai.Bag.InstrumentationMethod == m.Copy {
		jarPath = fmt.Sprintf("%s/%s", jarPath, "AppServerAgent")
	}

	bth := ai.AppdController.StartBT("Instrument")
	cmd := fmt.Sprintf("java -Xbootclasspath/a:%s/tools.jar -jar %s/javaagent.jar %d appdynamics.controller.hostName=%s,appdynamics.controller.port=%d,appdynamics.controller.ssl.enabled=%t,appdynamics.agent.accountName=%s,appdynamics.agent.accountAccessKey=%s,appdynamics.agent.applicationName=%s,appdynamics.agent.tierName=%s,appdynamics.agent.reuse.nodeName=true,appdynamics.agent.reuse.nodeName.prefix=%s",
		jdkPath, jarPath, pid, ai.Bag.ControllerUrl, ai.Bag.ControllerPort, ai.Bag.SSLEnabled, ai.Bag.Account, ai.Bag.AccessKey, appName, tierName, tierName)

	//BIQ instrumentation. If Analytics agent is remote, provide the url when attaching
	fmt.Printf("BiQ deployment option is %s.\n", biQDeploymentOption)
	if biQDeploymentOption == m.Remote {
		fmt.Printf("Will add remote url %s\n", ai.Bag.AnalyticsAgentUrl)
		if ai.Bag.AnalyticsAgentUrl != "" {
			cmd = fmt.Sprintf("%s,appdynamics.analytics.agent.url=%s/v2/sinks/bt", cmd, ai.Bag.AnalyticsAgentUrl)
		}
	}
	code, output, err := exec.RunCommandInPod(podObj.Name, podObj.Namespace, containerName, "", cmd)
	if code == 0 {
		fmt.Printf("AppD agent attached. Output: %s. Error: %v\n", output, err)

		//annotate pod
		if podObj.Annotations == nil {
			podObj.Annotations = make(map[string]string)
		}
		//annotate pod
		podObj.Annotations[ATTACHED_ANNOTATION] = time.Now().String()

		//determine the AppD node name
		findVerCmd := fmt.Sprintf("find %s/ -maxdepth 1 -type d -name '*ver*' -printf %%f -quit", jarPath)
		cVer, verFolderName, errVer := exec.RunCommandInPod(podObj.Name, podObj.Namespace, containerName, "", findVerCmd)
		fmt.Printf("AppD agent version probe. Output: %s. Error: %v\n", verFolderName, errVer)
		if cVer == 0 && verFolderName != "" {
			findCmd := fmt.Sprintf("find %s/%s/logs/ -maxdepth 1 -type d -name '*%s*' -printf %%f -quit", jarPath, verFolderName, tierName)
			c, folderName, errFolder := exec.RunCommandInPod(podObj.Name, podObj.Namespace, containerName, "", findCmd)
			fmt.Printf("AppD agent version probe. Output: %s. Error: %v\n", folderName, errFolder)
			if c == 0 {
				nodeName := strings.TrimSpace(folderName)
				podObj.Annotations[APPD_NODENAME] = nodeName
				appID, tierID, nodeID, errAppd := ai.AppdController.DetermineNodeID(appName, nodeName)
				if errAppd == nil {
					podObj.Annotations[APPD_APPID] = strconv.Itoa(appID)
					podObj.Annotations[APPD_TIERID] = strconv.Itoa(tierID)
					podObj.Annotations[APPD_NODEID] = strconv.Itoa(nodeID)
				}
			}
		}
		_, updateErr := ai.ClientSet.Core().Pods(podObj.Namespace).Update(podObj)
		if updateErr != nil {
			return fmt.Errorf("%s, Error: %v\n", ANNOTATION_UPDATE_ERROR, err)
		}
	} else {
		return fmt.Errorf("Unable to attach Java agent. Error code = %d. Output: %s, Error: %v\n", code, output, err)
	}
	ai.AppdController.StopBT(bth)
	return nil
}
