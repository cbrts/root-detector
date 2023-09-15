package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

// ContainerInfo stores information about a container.
type ContainerInfo struct {
	Namespace   string
	PodName     string
	Container   string
	CommandExec string
}

// authenticateToCluster authenticates to the Kubernetes cluster and returns a clientset and config.
func authenticateToCluster() (*kubernetes.Clientset, *rest.Config, error) {
	kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return clientset, config, nil
}

// listNamespaces lists all non-excluded namespaces in the cluster.
func listNamespaces(clientset *kubernetes.Clientset, excludeNamespaces []string) ([]string, error) {
	namespaces := []string{}

	namespaceList, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaceList.Items {
		// Check if the namespace is in the list of excluded namespaces
		if !containsString(excludeNamespaces, ns.Name) {
			namespaces = append(namespaces, ns.Name)
		}
	}

	return namespaces, nil
}

// containsString checks if a string is present in a slice of strings.
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// listPods lists all pods in the specified namespace.
func listPods(clientset *kubernetes.Clientset, namespace string) ([]string, error) {
	podNames := []string{}

	podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		podNames = append(podNames, pod.Name)
	}

	return podNames, nil
}

// listContainers lists all containers in the specified pod.
func listContainers(clientset *kubernetes.Clientset, namespace, podName string) ([]string, error) {
	containerNames := []string{}

	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	for _, container := range pod.Spec.Containers {
		containerNames = append(containerNames, container.Name)
	}

	return containerNames, nil
}

// execCommandInContainer executes a command in the specified container and returns the output.
func execCommandInContainer(clientset *kubernetes.Clientset, config *rest.Config, namespace, podName, containerName, command string) (string, error) {
	cmd := []string{
		"sh",
		"-c",
		command,
	}
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Command:   cmd,
		Container: containerName,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false, // TTY is set to false to ensure command output is captured properly
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", err
	}

	var stdout, stderr strings.Builder
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", err
	}

	return stdout.String(), nil
}

// findContainersWithErrors finds root containers and lists containers where the command errored based on the specified criteria.
func findContainersWithErrors(clientset *kubernetes.Clientset, config *rest.Config) ([]ContainerInfo, []ContainerInfo, error) {
	var rootContainers []ContainerInfo
	var errorContainers []ContainerInfo

	excludeNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}

	namespaces, err := listNamespaces(clientset, excludeNamespaces)
	if err != nil {
		return nil, nil, err
	}

	for _, namespace := range namespaces {
		pods, err := listPods(clientset, namespace)
		if err != nil {
			fmt.Printf("Error listing pods in namespace %s: %v\n", namespace, err)
			continue
		}

		for _, pod := range pods {
			containers, err := listContainers(clientset, namespace, pod)
			if err != nil {
				fmt.Printf("Error listing containers in pod %s: %v\n", pod, err)
				continue
			}

			for _, container := range containers {
				command := "whoami"
				output, err := execCommandInContainer(clientset, config, namespace, pod, container, command)
				if err != nil {
					fmt.Printf("Error running 'whoami' command in container %s: %v\n", container, err)
					errorContainers = append(errorContainers, ContainerInfo{
						Namespace:   namespace,
						PodName:     pod,
						Container:   container,
						CommandExec: command,
					})
					continue
				}

				if strings.Contains(output, "root") {
					rootContainers = append(rootContainers, ContainerInfo{
						Namespace:   namespace,
						PodName:     pod,
						Container:   container,
						CommandExec: command,
					})
				}
			}
		}
	}

	return rootContainers, errorContainers, nil
}

func main() {
	clientset, config, err := authenticateToCluster()
	if err != nil {
		fmt.Printf("Error authenticating to the cluster: %v\n", err)
		os.Exit(1)
	}

	rootContainers, errorContainers, err := findContainersWithErrors(clientset, config)
	if err != nil {
		fmt.Printf("Error finding containers: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nRoot Containers:")
	for _, rootContainer := range rootContainers {
		fmt.Printf("Namespace: %s, Pod: %s, Container: %s, CommandExec: %s\n", rootContainer.Namespace, rootContainer.PodName, rootContainer.Container, rootContainer.CommandExec)
	}

	fmt.Println("\nContainers with Errors:")
	for _, errorContainer := range errorContainers {
		fmt.Printf("Namespace: %s, Pod: %s, Container: %s, CommandExec: %s\n", errorContainer.Namespace, errorContainer.PodName, errorContainer.Container, errorContainer.CommandExec)
	}
}
