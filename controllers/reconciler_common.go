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

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-lib/status"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openstack-k8s-operators/ovn-central-operator/util"
)

type ReconcilerCommon interface {
	GetClient() client.Client
	GetLogger() logr.Logger
}

func WrapErrorForObject(msg string, object runtime.Object, err error) error {
	key, keyErr := client.ObjectKeyFromObject(object)
	if keyErr != nil {
		return fmt.Errorf("ObjectKeyFromObject %v: %w", object, keyErr)
	}

	return fmt.Errorf("%s %T %v: %w",
		msg, object, key, err)
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

func UpdateStatus(r ReconcilerCommon,
	ctx context.Context,
	object runtime.Object,
	errMsg string,
	params ...interface{}) (ctrl.Result, error) {

	if err := r.GetClient().Status().Update(ctx, object); err != nil {
		err = WrapErrorForObject(errMsg, object, err)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func DeleteIfExists(r ReconcilerCommon,
	ctx context.Context, obj runtime.Object) error {

	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		err = WrapErrorForObject("ObjectKeyFromObject", obj, err)
		return err
	}

	err = r.GetClient().Get(ctx, key, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		err = WrapErrorForObject("Get", obj, err)
		return err
	}

	err = r.GetClient().Delete(ctx, obj)
	if err != nil {
		err = WrapErrorForObject("Delete", obj, err)
		return err
	}

	accessor := getAccessorOrDie(obj)
	LogForObject(r, "Delete", accessor)
	return nil
}

func NeedsUpdate(
	r ReconcilerCommon,
	ctx context.Context,
	obj runtime.Object,
	f controllerutil.MutateFn) (bool, error) {

	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		err = WrapErrorForObject("ObjectKeyFromObject", obj, err)
		return false, err
	}

	if err := r.GetClient().Get(ctx, key, obj); err != nil {
		if errors.IsNotFound(err) {
			return true, nil
		} else {
			err = WrapErrorForObject("Get", obj, err)
			return false, err
		}
	}

	existing := obj.DeepCopyObject()
	if err := f(); err != nil {
		return false, err
	}

	return !equality.Semantic.DeepEqual(existing, obj), nil
}

func CreateOrDelete(
	r ReconcilerCommon,
	ctx context.Context,
	obj runtime.Object,
	f controllerutil.MutateFn) (controllerutil.OperationResult, error) {

	key, err := client.ObjectKeyFromObject(obj)
	if err != nil {
		err = WrapErrorForObject("ObjectKeyFromObject", obj, err)
		return controllerutil.OperationResultNone, err
	}

	if err := r.GetClient().Get(ctx, key, obj); err != nil {
		if errors.IsNotFound(err) {
			if err := f(); err != nil {
				err = WrapErrorForObject("Initialise", obj, err)
				return controllerutil.OperationResultNone, err
			}

			if err := r.GetClient().Create(ctx, obj); err != nil {
				err = WrapErrorForObject("Create", obj, err)
				return controllerutil.OperationResultNone, err
			}

			return controllerutil.OperationResultCreated, nil
		} else {
			err = WrapErrorForObject("Get", obj, err)
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
		err = WrapErrorForObject("Delete", obj, err)
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

func CheckConditions(
	r ReconcilerCommon, ctx context.Context,
	obj util.RuntimeObjectWithConditions, origConditions status.Conditions, err *error) {

	if !reflect.DeepEqual(obj.GetConditions(), &origConditions) {
		if updateErr := r.GetClient().Status().Update(ctx, obj); updateErr != nil {
			if *err == nil {
				// Return the update error if Reconcile() isn't already returning an
				// error
				*err = WrapErrorForObject("Update Status", obj, updateErr)
			} else {
				// Reconciler() is already returning an error, so log this error but
				// leave the original unchanged
				accessor := getAccessorOrDie(obj)
				LogErrorForObject(r, updateErr, "Update", accessor)
			}
		}
	}
}
