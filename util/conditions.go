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
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ObjectWithConditions interface {
	GetConditions() *condition.Conditions
}

type RuntimeObjectWithConditions interface {
	ObjectWithConditions
	runtime.Object
}

const (
	ConditionAvailable   condition.Type = "Available"
	ConditionFailed      condition.Type = "Failed"
	ConditionInitialized condition.Type = "Initialized"
)

func SetAvailable(obj ObjectWithConditions) {
	condition := condition.Condition{
		Type:   ConditionAvailable,
		Status: corev1.ConditionTrue,
	}

	obj.GetConditions().Set(&condition)
}

func UnsetAvailable(obj ObjectWithConditions) {
	//	obj.GetConditions().Set(ConditionAvailable)
}

func IsAvailable(obj ObjectWithConditions) bool {
	return obj.GetConditions().IsTrue(ConditionAvailable)
}

func SetFailed(obj ObjectWithConditions, reason condition.Reason, msg string) {
	condition := condition.Condition{
		Type:    ConditionFailed,
		Status:  corev1.ConditionTrue,
		Reason:  reason,
		Message: msg,
	}

	obj.GetConditions().Set(&condition)
}

func UnsetFailed(obj ObjectWithConditions) {
	//	obj.GetConditions().Set(ConditionFailed)
}

func IsFailed(obj ObjectWithConditions) bool {
	return obj.GetConditions().IsTrue(ConditionFailed)
}

func SetInitialized(obj ObjectWithConditions) {
	condition := condition.Condition{
		Type:   ConditionInitialized,
		Status: corev1.ConditionTrue,
	}

	obj.GetConditions().Set(&condition)
}

func UnsetInitialized(obj ObjectWithConditions) {
	//	obj.GetConditions().Set(ConditionInitialized)
}

func IsInitialized(obj ObjectWithConditions) bool {
	return obj.GetConditions().IsTrue(ConditionInitialized)
}

func DeepCopyConditions(conditions condition.Conditions) condition.Conditions {
	cp := make(condition.Conditions, len(conditions))
	for i, condition := range conditions {
		condition.DeepCopyInto(&cp[i])
	}

	return cp
}
