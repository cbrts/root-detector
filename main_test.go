package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var testPodName = "test-busybox-pod"
var randomNamespace = "test-" + uuid.New().String()

func setupTest() (*kubernetes.Clientset, *rest.Config, error) {
	// Authenticate to the Kubernetes cluster
	clientset, config, err := authenticateToCluster()
	if err != nil {
		return nil, nil, err
	}

	// Create a test namespace
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: randomNamespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}

	// Create a test BusyBox pod in the test namespace
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: randomNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"}, // Sleep to keep the pod running
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	_, err = clientset.CoreV1().Pods(randomNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}

	// Wait for the pod to be ready
	err = waitForPodReady(clientset, randomNamespace, testPodName, time.Second*10)
	if err != nil {
		return nil, nil, err
	}

	return clientset, config, nil
}

func waitForPodReady(clientset *kubernetes.Clientset, namespace, podName string, timeout time.Duration) error {
	// Create a watcher for the pod
	watcher, err := clientset.CoreV1().Pods(namespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + podName,
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	// Wait for the pod to be ready or for a timeout
	timeoutCh := time.After(timeout)
	for {
		select {
		case event := <-watcher.ResultChan():
			pod, ok := event.Object.(*v1.Pod)
			if !ok {
				continue
			}
			if pod.Status.Phase == v1.PodRunning {
				return nil
			}
		case <-timeoutCh:
			return fmt.Errorf("timeout waiting for pod to be ready")
		}
	}
}

func TestMain(m *testing.M) {
	// Setup function for tests
	clientset, _, err := setupTest()
	if err != nil {
		fmt.Printf("Error setting up tests: %v\n", err)
		os.Exit(1)
	}

	// Run the tests
	code := m.Run()

	// Cleanup resources (delete the test namespace) regardless of test outcome
	err = clientset.CoreV1().Namespaces().Delete(context.TODO(), randomNamespace, metav1.DeleteOptions{})
	if err != nil {
		fmt.Printf("Error cleaning up test resources: %v\n", err)
	}

	os.Exit(code)
}

func TestAuthenticateToCluster(t *testing.T) {
	// Run the authentication function
	clientset, config, err := authenticateToCluster()

	// Check if there was an error during authentication
	assert.NoError(t, err)

	// Check if the clientset and config are not nil
	assert.NotNil(t, clientset)
	assert.NotNil(t, config)
}

func TestListNamespaces(t *testing.T) {
	// Authenticate to the Kubernetes cluster
	clientset, _, err := authenticateToCluster()
	assert.NoError(t, err)

	// Define namespaces to exclude from the list
	excludeNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}

	// Call the listNamespaces function
	namespaces, err := listNamespaces(clientset, excludeNamespaces)

	// Check for errors in the listNamespaces function
	assert.NoError(t, err)

	// Check if the list of namespaces is not nil
	assert.NotNil(t, namespaces)

	// Check that excluded namespaces are not in the list
	assert.NotContains(t, namespaces, "kube-system")
	assert.NotContains(t, namespaces, "kube-public")
	assert.NotContains(t, namespaces, "kube-node-lease")

}

func TestContainsString(t *testing.T) {
	// Define a slice of strings to search in
	slice := []string{"apple", "banana", "cherry", "date"}

	// Test cases
	testCases := []struct {
		input    string
		expected bool
	}{
		{"apple", true},
		{"banana", true},
		{"cherry", true},
		{"date", true},
		{"grape", false},
	}

	// Run test cases
	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("Input: %s, Expected: %v", testCase.input, testCase.expected), func(t *testing.T) {
			result := containsString(slice, testCase.input)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestListPods(t *testing.T) {
	// Authenticate to the Kubernetes cluster
	clientset, _, err := authenticateToCluster()
	assert.NoError(t, err)

	// Define a test namespace
	namespace := randomNamespace

	// Call the listPods function
	pods, err := listPods(clientset, namespace)

	// Check for errors in the listPods function
	assert.NoError(t, err)

	// Check if the list of pods is not nil
	assert.NotNil(t, pods)

	// Check that the test pod is in the list
	assert.Contains(t, pods, testPodName)
}

func TestListContainers(t *testing.T) {
	// Authenticate to the Kubernetes cluster and create the clientset
	clientset, _, err := authenticateToCluster()
	assert.NoError(t, err)

	// Define a test namespace and pod name
	namespace := randomNamespace
	podName := testPodName

	// Call the listContainers function with the clientset
	containers, err := listContainers(clientset, namespace, podName)

	// Check for errors in the listContainers function
	assert.NoError(t, err)

	// Check if the list of containers is not nil
	assert.NotNil(t, containers)

	// Check that the test container is in the list (in this case, "busybox")
	assert.Contains(t, containers, "busybox")
}

func TestExecCommandInContainer(t *testing.T) {
	// Authenticate to the Kubernetes cluster and create the clientset
	clientset, config, err := authenticateToCluster()
	assert.NoError(t, err)

	// Define test parameters
	namespace := randomNamespace
	podName := testPodName
	containerName := "busybox"
	command := "echo 'Hello, Test!'"

	// Call the execCommandInContainer function
	output, err := execCommandInContainer(clientset, config, namespace, podName, containerName, command)

	// Check for errors in the execCommandInContainer function
	assert.NoError(t, err)

	// Check if the output is not empty and contains the expected string
	assert.NotNil(t, output)
	assert.Contains(t, output, "Hello, Test!")
}

func TestFindContainersWithErrors(t *testing.T) {
	// Authenticate to the Kubernetes cluster and create the clientset
	clientset, config, err := authenticateToCluster()
	assert.NoError(t, err)

	// Call the findContainersWithErrors function
	rootContainers, errorContainers, err := findContainersWithErrors(clientset, config)

	// Check for errors in the findContainersWithErrors function
	assert.NoError(t, err)

	// Check if the lists of root containers and error containers are not nil
	assert.NotNil(t, rootContainers)
	assert.Nil(t, errorContainers)
}
