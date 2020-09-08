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

package controllers

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-test/deep"
)

type ApplyType string

const (
	ApplyTypeNone   ApplyType = "None"
	ApplyTypeCreate           = "Create"
	ApplyTypeUpdate           = "Update"
)

func (at *ApplyType) String() string {
	return string(*at)
}

type k8sObject interface {
	metav1.Object
	runtime.Object
}

func (r *OVNCentralReconciler) fetchOrCreateObject(
	ctx context.Context,
	object, fetched k8sObject) (k8sObject, error) {

	err := r.Client.Get(ctx,
		types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()},
		fetched)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Object doesn't exist. Create it.
			err = r.Client.Create(ctx, object)
			if err != nil {
				err = WrapErrorForObject("Create", object, err)
				return object, err
			}
			return object, nil
		}

		err = WrapErrorForObject("Get", object, err)
		return object, err
	}

	return fetched, nil
}

func reconcileLabels(fetched, template metav1.Object) (updated bool) {

	// Check fetched object has all labels on template object (we ignore additional labels)
	fetchedLabels := fetched.GetLabels()
	for label, value := range template.GetLabels() {
		fvalue, ok := fetchedLabels[label]
		if !ok || fvalue != value {
			fetchedLabels[label] = value
			updated = true
		}
	}
	if updated {
		fetched.SetLabels(fetchedLabels)
	}
	return
}

func reconcileOwners(fetched, template metav1.Object) (updated bool) {
	fetchedOwners := fetched.GetOwnerReferences()
	for _, owner := range template.GetOwnerReferences() {
		var found bool
		for _, fetchedOwner := range fetchedOwners {
			if reflect.DeepEqual(fetchedOwner, owner) {
				found = true
				break
			}
		}
		if !found {
			updated = true
			fetchedOwners = append(fetchedOwners, owner)
		}
	}
	if updated {
		fetched.SetOwnerReferences(fetchedOwners)
	}
	return
}

func (r *OVNCentralReconciler) ApplyService(
	ctx context.Context,
	object *corev1.Service) (*corev1.Service, error) {

	fetched := &corev1.Service{}
	fetched_obj, err := r.fetchOrCreateObject(ctx, object, fetched)
	fetched, ok := fetched_obj.(*corev1.Service)
	if !ok {
		panic(fmt.Errorf("fetchOrCreateObject returned %T, expected %T",
			fetched_obj, fetched))
	}
	if err != nil {
		return fetched, err
	}

	var updated bool

	if reconcileLabels(fetched, object) {
		r.LogForObject("Labels differ", object)
		updated = true
	}

	if reconcileOwners(fetched, object) {
		r.LogForObject("Owners differ", object)
		updated = true
	}

	// Set the target ClusterIP to the value we fetched from the server. This value is set
	// server-side, and can't be updated.
	object.Spec.ClusterIP = fetched.Spec.ClusterIP

	// We didn't explicitly set TargetPort or Protocol for ports, so they were defaulted
	// server-side
	for _, port := range fetched.Spec.Ports {
		for i, origPort := range object.Spec.Ports {
			if origPort.Name == port.Name {
				object.Spec.Ports[i].TargetPort = port.TargetPort
				object.Spec.Ports[i].Protocol = port.Protocol
				break
			}
		}
	}

	// We only directly compare the Specs of the 2 objects
	if diff := deep.Equal(object.Spec, fetched.Spec); diff != nil {
		fetched.Spec = object.Spec
		r.LogForObject("Specs differ", object, "ObjectDiff", diff)
		updated = true
	}

	if updated {
		err = r.Client.Update(ctx, fetched)
		if err != nil {
			err = WrapErrorForObject("Update", fetched, err)
			return fetched, err
		}
		return fetched, nil
	}

	return fetched, nil
}

func (r *OVNCentralReconciler) ApplyPVC(
	ctx context.Context,
	object *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {

	fetched := &corev1.PersistentVolumeClaim{}
	fetched_obj, err := r.fetchOrCreateObject(ctx, object, fetched)
	fetched, ok := fetched_obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		panic(fmt.Errorf("fetchOrCreateObject returned %T, expected %T",
			fetched_obj, fetched))
	}
	if err != nil {
		return fetched, err
	}

	var updated bool

	if reconcileLabels(fetched, object) {
		r.LogForObject("Labels differ", object)
		updated = true
	}

	if reconcileOwners(fetched, object) {
		r.LogForObject("Owners differ", object)
		updated = true
	}

	// PVC Spec is immutable

	if updated {
		err = r.Client.Update(ctx, fetched)
		if err != nil {
			err = WrapErrorForObject("Update", fetched, err)
			return fetched, err
		}
	}

	return fetched, nil
}
