/*
Copyright 2020 Red Hat

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/exec"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type ExecResult struct {
	Stdout     *bufio.Reader
	Stderr     *bufio.Reader
	ExitStatus int
}

var restConfig *rest.Config

func init() {
	restConfig = config.GetConfigOrDie()
}

func GetLogStream(ctx context.Context,
	pod *corev1.Pod,
	container string,
	limit int64) (io.ReadCloser, error) {

	// mdbooth: AFAICT it is not possible to read pod logs using the
	// controller-runtime client
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		err := fmt.Errorf("NewForConfig: %w", err)
		return nil, err
	}

	podLogOpts := corev1.PodLogOptions{Container: container, LimitBytes: &limit}
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)
	return req.Stream(ctx)
}

func IsPodConditionSet(conditionType corev1.PodConditionType, pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func PodExec(pod *corev1.Pod, containerName string, command []string) (*ExecResult, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("Initialising clientset: %w", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   command,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("Initialising executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	result := &ExecResult{
		Stdout: bufio.NewReader(&stdout),
		Stderr: bufio.NewReader(&stderr),
	}
	err = executor.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		if exitError, ok := err.(exec.ExitError); ok {
			result.ExitStatus = exitError.ExitStatus()
		} else {
			return nil, fmt.Errorf("Executing remote command: %w", err)
		}
	}

	return result, nil
}
