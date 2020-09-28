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
	"github.com/operator-framework/operator-lib/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ObjectWithConditions interface {
	GetConditions() *status.Conditions
}

type RuntimeObjectWithConditions interface {
	ObjectWithConditions
	runtime.Object
}

const (
	ConditionAvailable   status.ConditionType = "Available"
	ConditionFailed      status.ConditionType = "Failed"
	ConditionInitialized status.ConditionType = "Initialized"
)

func SetAvailable(obj ObjectWithConditions) {
	condition := status.Condition{
		Type:   ConditionAvailable,
		Status: corev1.ConditionTrue,
	}

	obj.GetConditions().SetCondition(condition)
}

func UnsetAvailable(obj ObjectWithConditions) {
	obj.GetConditions().RemoveCondition(ConditionAvailable)
}

func IsAvailable(obj ObjectWithConditions) bool {
	return obj.GetConditions().IsTrueFor(ConditionAvailable)
}

func SetFailed(obj ObjectWithConditions, reason status.ConditionReason, msg string) {
	condition := status.Condition{
		Type:    ConditionFailed,
		Status:  corev1.ConditionTrue,
		Reason:  reason,
		Message: msg,
	}

	obj.GetConditions().SetCondition(condition)
}

func UnsetFailed(obj ObjectWithConditions) {
	obj.GetConditions().RemoveCondition(ConditionFailed)
}

func IsFailed(obj ObjectWithConditions) bool {
	return obj.GetConditions().IsTrueFor(ConditionFailed)
}

func SetInitialized(obj ObjectWithConditions) {
	condition := status.Condition{
		Type:   ConditionInitialized,
		Status: corev1.ConditionTrue,
	}

	obj.GetConditions().SetCondition(condition)
}

func UnsetInitialized(obj ObjectWithConditions) {
	obj.GetConditions().RemoveCondition(ConditionInitialized)
}

func IsInitialized(obj ObjectWithConditions) bool {
	return obj.GetConditions().IsTrueFor(ConditionInitialized)
}

func DeepCopyConditions(conditions status.Conditions) status.Conditions {
	cp := make(status.Conditions, len(conditions))
	for i, condition := range conditions {
		condition.DeepCopyInto(&cp[i])
	}

	return cp
}
