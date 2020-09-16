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
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func GetLogStream(ctx context.Context,
	pod *corev1.Pod,
	container string,
	limit int64) (io.ReadCloser, error) {

	// mdbooth: AFAICT it is not possible to read pod logs using the
	// controller-runtime client
	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
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
