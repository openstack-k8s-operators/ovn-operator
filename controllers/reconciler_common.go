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
	"k8s.io/apimachinery/pkg/api/equality"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ReconcilerCommon interface {
	GetClient() client.Client
	GetLogger() logr.Logger
}

func WrapErrorForObject(msg string, object metav1.Object, err error) error {
	return fmt.Errorf("%s %T %v/%v: %w",
		msg, object, object.GetNamespace(), object.GetName(), err)
}

func logObjectParams(object metav1.Object) []interface{} {
	return []interface{}{
		"ObjectType", fmt.Sprintf("%T", object),
		"ObjectNamespace", object.GetNamespace(),
		"ObjectName", object.GetName()}
}

func LogForObject(r ReconcilerCommon,
	msg string, object metav1.Object, params ...interface{}) {

	params = append(params, logObjectParams(object)...)
	r.GetLogger().Info(msg, params...)
}

func LogErrorForObject(r ReconcilerCommon,
	err error, msg string, object metav1.Object, params ...interface{}) {

	params = append(params, logObjectParams(object)...)
	r.GetLogger().Error(err, msg, params...)
}

func DeleteIfExists(r ReconcilerCommon,
	ctx context.Context, obj runtime.Object) error {

	accessor := getAccessorOrDie(obj)
	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		err = WrapErrorForObject("ObjectKeyFromObject", accessor, err)
		return err
	}

	err = r.GetClient().Get(ctx, key, obj)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		err = WrapErrorForObject("Get", accessor, err)
		return err
	}

	err = r.GetClient().Delete(ctx, obj)
	if err != nil {
		err = WrapErrorForObject("Delete", accessor, err)
		return err
	}

	LogForObject(r, "Delete", accessor)
	return nil
}

func CreateOrDelete(
	r ReconcilerCommon,
	ctx context.Context,
	obj runtime.Object,
	f controllerutil.MutateFn) (controllerutil.OperationResult, error) {

	accessor := getAccessorOrDie(obj)

	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		err = WrapErrorForObject("ObjectKeyFromObject", accessor, err)
		return controllerutil.OperationResultNone, err
	}

	if err := r.GetClient().Get(ctx, key, obj); err != nil {
		if k8s_errors.IsNotFound(err) {
			if err := f(); err != nil {
				err = WrapErrorForObject("Initialise", accessor, err)
				return controllerutil.OperationResultNone, err
			}

			if err := r.GetClient().Create(ctx, obj); err != nil {
				err = WrapErrorForObject("Create", accessor, err)
				return controllerutil.OperationResultNone, err
			}

			return controllerutil.OperationResultCreated, nil
		} else {
			err = WrapErrorForObject("Get", accessor, err)
			return controllerutil.OperationResultNone, err
		}
	}

	existing := obj.DeepCopyObject()
	if err := f(); err != nil {
		return controllerutil.OperationResultNone, err
	}

	if equality.Semantic.DeepEqual(existing, obj) {
		return controllerutil.OperationResultNone, nil
	}

	// This is really useful for debugging differences. Import github.com/go-test/deep
	//diff := deep.Equal(existing, obj)
	//LogForObject(r, "Objects differ", accessor, "ObjectDiff", diff)

	if err := r.GetClient().Delete(ctx, obj); err != nil {
		err = WrapErrorForObject("Delete", accessor, err)
		return controllerutil.OperationResultNone, nil
	}

	return controllerutil.OperationResultUpdated, nil
}

func getAccessorOrDie(obj runtime.Object) metav1.Object {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		// Programming error: obj is of the wrong type
		panic(fmt.Errorf("Unable to get accessor for object %v: %w", obj, err))
	}

	return accessor
}
