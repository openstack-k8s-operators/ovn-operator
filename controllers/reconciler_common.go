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

	"github.com/go-logr/logr"

	//"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ReconcilerCommon -
type ReconcilerCommon interface {
	GetClient() client.Client
	GetLogger() logr.Logger
}

// WrapErrorForObject -
func WrapErrorForObject(msg string, object client.Object, err error) error {
	key := client.ObjectKeyFromObject(object)

	return fmt.Errorf("%s %T %v: %w",
		msg, object, key, err)
}

func logObjectParams(object metav1.Object) []interface{} {
	return []interface{}{
		"ObjectType", fmt.Sprintf("%T", object),
		"ObjectNamespace", object.GetNamespace(),
		"ObjectName", object.GetName()}
}

// LogForObject -
func LogForObject(r ReconcilerCommon,
	msg string, object metav1.Object, params ...interface{}) {

	params = append(params, logObjectParams(object)...)
	r.GetLogger().Info(msg, params...)
}

// LogErrorForObject -
func LogErrorForObject(r ReconcilerCommon,
	err error, msg string, object metav1.Object, params ...interface{}) {

	params = append(params, logObjectParams(object)...)
	r.GetLogger().Error(err, msg, params...)
}

// CreateOrDelete -
func CreateOrDelete(
	ctx context.Context,
	r ReconcilerCommon,
	obj client.Object,
	f controllerutil.MutateFn) (controllerutil.OperationResult, error) {

	accessor := getAccessorOrDie(obj)

	op, err := controllerutil.CreateOrUpdate(ctx, r.GetClient(), obj, f)
	if err != nil && errors.IsInvalid(err) {
		// Request to make an unsupported change
		if err := r.GetClient().Delete(ctx, obj); err != nil {
			err = WrapErrorForObject("Delete", obj, err)
			return op, err
		}

		accessor := getAccessorOrDie(obj)
		LogForObject(r, "Deleted", accessor)
		return controllerutil.OperationResultUpdated, nil
	}

	if op != controllerutil.OperationResultNone {
		LogForObject(r, "Updated", accessor)
	}
	return op, err
}

func getAccessorOrDie(obj runtime.Object) metav1.Object {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		// Programming error: obj is of the wrong type
		panic(fmt.Errorf("Unable to get accessor for object %v: %w", obj, err))
	}

	return accessor
}
