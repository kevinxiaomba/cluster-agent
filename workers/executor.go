package workers

import (
	"fmt"
	"io"
	"os"
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

type Executor struct {
	ClientSet *kubernetes.Clientset
	K8sConfig *rest.Config
}

const (
	DEFAULT_EXEC_CMD string = "/bin/sh"
	ERROR_CODE       int    = -1
)

func NewExecutor(clientSet *kubernetes.Clientset, config *rest.Config) Executor {
	return Executor{clientSet, config}
}

func (exec *Executor) RunCommand(podObject *v1.Pod, cmd string, args string) (int, string, error) {

	if len(podObject.Spec.Containers) == 0 {
		return ERROR_CODE, "", fmt.Errorf("Pod must have at least 1 container to execute commnads")
	}
	containerName := podObject.Spec.Containers[0].Name
	return exec.RunCommandInPod(podObject.Name, podObject.Namespace, containerName, cmd, args)
}

func (exec *Executor) RunCommandInPod(podName string, namespace string, containerName string, cmd string, args string) (int, string, error) {
	return exec.execute(podName, namespace, containerName, cmd, args)
}

func (exec *Executor) execute(podName string, namespace string, containerName string, cmd string, args string) (int, string, error) {
	if cmd == "" {
		cmd = DEFAULT_EXEC_CMD
	}
	fmt.Printf("About to exec command %s in pod %s/%s, container %s\n", cmd, namespace, podName, containerName)
	var execOut Writer
	var execErr Writer

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", namespace, podName)
	execRequest := exec.ClientSet.RESTClient().Post().AbsPath(path)
	execRequest.Param("command", cmd).Param("command", "-c").Param("command", args).Param("container", containerName).Param("stdin", "true").Param("stderr", "true").Param("stdout", "true").Param("tty", "false")
	command, err := remotecommand.NewSPDYExecutor(exec.K8sConfig, "POST", execRequest.URL())

	if err != nil {
		fmt.Printf("Issues creating command. %v\n", err)
		return ERROR_CODE, "", err
	}
	fmt.Println("Request URL:", execRequest.URL().String())

	cmdErr := command.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})
	var exitCode int

	if cmdErr == nil {
		exitCode = 0
		fmt.Printf("Command executed successfully. Status %d. Output: %s\n", exitCode, execOut.GetOutput())
	} else {
		fmt.Printf("Command returned an error. %v\n", cmdErr)
		if exitErr, ok := cmdErr.(utilexec.ExitError); ok && exitErr.Exited() {
			exitCode = exitErr.ExitStatus()
			fmt.Printf("Exit status: %d\n", exitCode)
			return exitCode, execErr.GetOutput(), exitErr
		} else {
			fmt.Printf("Issues getting exit code: %v, %v\n", exitErr, cmdErr)
			return exitCode, execErr.GetOutput(), fmt.Errorf("Failed to find exit code.")
		}
	}

	return exitCode, execOut.GetOutput(), nil
}

func newStringReader(ss []string) io.Reader {
	formattedString := strings.Join(ss, "\n")
	reader := strings.NewReader(formattedString)
	return reader
}

type Writer struct {
	Str []string
}

func (w *Writer) Write(p []byte) (n int, err error) {
	str := string(p)
	if len(str) > 0 {
		w.Str = append(w.Str, str)
	}
	return len(str), nil
}

func (w *Writer) GetOutput() string {
	return strings.Join(w.Str, "")
}
