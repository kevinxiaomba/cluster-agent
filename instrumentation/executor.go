package instrumentation

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

type Executor struct {
	ClientSet *kubernetes.Clientset
	K8sConfig *rest.Config
	Logger    *log.Logger
}

const (
	DEFAULT_EXEC_CMD string = "/bin/sh"
	ERROR_CODE       int    = -1
)

var (
	errFileSpecDoesntMatchFormat = errors.New("Filespec must match the canonical format: [[namespace/]pod:]file/path")
	errFileCannotBeEmpty         = errors.New("Filepath can not be empty")
)

type fileSpec struct {
	File          string
	PodName       string
	Namespace     string
	ContainerName string
}

func NewExecutor(clientSet *kubernetes.Clientset, config *rest.Config, l *log.Logger) Executor {
	return Executor{ClientSet: clientSet, K8sConfig: config, Logger: l}
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
	exec.Logger.Debugf("About to exec command %s %s in pod %s/%s, container %s\n", cmd, args, namespace, podName, containerName)
	var execOut Writer
	var execErr Writer

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", namespace, podName)
	execRequest := exec.ClientSet.RESTClient().Post().AbsPath(path)
	execRequest.Param("command", cmd).Param("command", "-c").Param("command", args).Param("container", containerName).Param("stdin", "true").Param("stderr", "true").Param("stdout", "true").Param("tty", "false")
	command, err := remotecommand.NewSPDYExecutor(exec.K8sConfig, "POST", execRequest.URL())

	if err != nil {
		exec.Logger.Errorf("Issues when creating command %s. %v\n", execRequest.URL().String(), err)
		return ERROR_CODE, "", err
	}

	cmdErr := command.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})
	var exitCode int

	if cmdErr == nil {
		exitCode = 0
		exec.Logger.Debugf("Command executed successfully. Status %d. Output: %s\n", exitCode, execOut.GetOutput())
	} else {
		exec.Logger.Errorf("Command %s returned an error. %v\n", execRequest.URL().String(), cmdErr)
		if exitErr, ok := cmdErr.(utilexec.ExitError); ok && exitErr.Exited() {
			exitCode = exitErr.ExitStatus()
			exec.Logger.Debugf("Exit status: %d\n", exitCode)
			return exitCode, execErr.GetOutput(), exitErr
		} else {
			exec.Logger.Errorf("Issues getting exit code: %v, %v\n", exitErr, cmdErr)
			return exitCode, execErr.GetOutput(), fmt.Errorf("Failed to find exit code.")
		}
	}

	return exitCode, execOut.GetOutput(), nil
}

func (exec *Executor) executeTar(podName string, namespace string, containerName string, cmd string, reader *io.PipeReader) (int, string, error) {

	fmt.Printf("About to exec command %s in pod %s/%s, container %s\n", cmd, namespace, podName, containerName)
	var execOut Writer
	var execErr Writer

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", namespace, podName)
	execRequest := exec.ClientSet.RESTClient().Post().AbsPath(path)
	execRequest.Param("command", DEFAULT_EXEC_CMD).Param("command", "-c").Param("command", cmd).Param("container", containerName).Param("stdin", "true").Param("stderr", "true").Param("stdout", "true").Param("tty", "false")
	command, err := remotecommand.NewSPDYExecutor(exec.K8sConfig, "POST", execRequest.URL())

	if err != nil {
		fmt.Printf("Issues creating command. %v\n", err)
		return ERROR_CODE, "", err
	}
	fmt.Println("Request URL:", execRequest.URL().String())

	cmdErr := command.Stream(remotecommand.StreamOptions{
		Stdin:  reader,
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

//file copy

func (exec *Executor) CopyFilesToPod(podObject *v1.Pod, containerName string, src string, dest string) (int, string, error) {
	if len(podObject.Spec.Containers) == 0 {
		return ERROR_CODE, "", fmt.Errorf("Pod must have at least 1 container to execute commnads")
	}

	srcFile := fileSpec{src, podObject.Name, podObject.Namespace, containerName}
	destFile := fileSpec{dest, podObject.Name, podObject.Namespace, containerName}
	return exec.copyToPod(podObject, containerName, srcFile, destFile)
}

func (exec *Executor) copyToPod(podObject *v1.Pod, containerName string, src, dest fileSpec) (int, string, error) {
	if len(src.File) == 0 {
		return -1, "", errFileCannotBeEmpty
	}
	reader, writer := io.Pipe()

	// strip trailing slash (if any)
	if strings.HasSuffix(string(dest.File[len(dest.File)-1]), "/") {
		dest.File = dest.File[:len(dest.File)-1]
	}

	fmt.Printf("Checking destination directory %s\n", dest.File)
	if errMakeDir := exec.makeDir(dest); errMakeDir == nil {
		dest.File = dest.File + "/" + path.Base(src.File)
		fmt.Printf("Checking directory %s is in place\n", dest.File)
	} else {
		fmt.Printf("Directory checked failed. v%\n", errMakeDir)
		return -1, "", errMakeDir
	}

	go func() {
		defer writer.Close()
		err := makeTar(src.File, dest.File, writer)
		if err != nil {
			fmt.Printf("Unable to tar file %s. %v\n", src.File, err)
		}
	}()

	cmdArr := []string{"tar", "xf", "-"}
	destDir := path.Dir(dest.File)
	if len(destDir) > 0 {
		cmdArr = append(cmdArr, "-C", destDir)
	}

	cmd := strings.Join(cmdArr, " ")
	return exec.executeTar(podObject.Name, podObject.Namespace, containerName, cmd, reader)
}

func (exec *Executor) checkDestinationIsDir(dest fileSpec) error {

	cmd := "test -d " + dest.File

	code, output, err := exec.execute(dest.PodName, dest.Namespace, dest.ContainerName, "", cmd)
	fmt.Printf("Destination file check returned with code %d. Output: %v, Error: %v\n", code, output, err)
	return err
}

func (exec *Executor) makeDir(dest fileSpec) error {

	cmd := "mkdir -p " + dest.File

	code, output, err := exec.execute(dest.PodName, dest.Namespace, dest.ContainerName, "", cmd)
	fmt.Printf("Make dest directory returned with code %d. Output: %v, Error: %v\n", code, output, err)
	return err
}

//tar

func makeTar(srcPath, destPath string, writer io.Writer) error {
	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	srcPath = path.Clean(srcPath)
	destPath = path.Clean(destPath)
	return recursiveTar(path.Dir(srcPath), path.Base(srcPath), path.Dir(destPath), path.Base(destPath), tarWriter)
}

func recursiveTar(srcBase, srcFile, destBase, destFile string, tw *tar.Writer) error {
	filepath := path.Join(srcBase, srcFile)
	stat, err := os.Lstat(filepath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		files, err := ioutil.ReadDir(filepath)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			//case empty directory
			hdr, _ := tar.FileInfoHeader(stat, filepath)
			hdr.Name = destFile
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
		}
		for _, f := range files {
			if err := recursiveTar(srcBase, path.Join(srcFile, f.Name()), destBase, path.Join(destFile, f.Name()), tw); err != nil {
				return err
			}
		}
		return nil
	} else if stat.Mode()&os.ModeSymlink != 0 {
		//case soft link
		hdr, _ := tar.FileInfoHeader(stat, filepath)
		target, err := os.Readlink(filepath)
		if err != nil {
			return err
		}

		hdr.Linkname = target
		hdr.Name = destFile
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
	} else {
		//case regular file or other file type like pipe
		hdr, err := tar.FileInfoHeader(stat, filepath)
		if err != nil {
			return err
		}
		hdr.Name = destFile

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		f, err := os.Open(filepath)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
		return f.Close()
	}
	return nil
}
