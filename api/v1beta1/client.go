/*
Copyright 2022.

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

package v1beta1

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// GetDBEndpoints - get DB Endpoints
func GetDBEndpoints(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	labelSelector map[string]string,
) (map[string]string, error) {
	ovnDBList := &OVNDBClusterList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
	}

	if len(labelSelector) > 0 {
		labels := client.MatchingLabels(labelSelector)
		listOpts = append(listOpts, labels)
	}

	err := h.GetClient().List(ctx, ovnDBList, listOpts...)
	if err != nil {
		return nil, err
	}

	if len(ovnDBList.Items) > 2 {
		return nil, fmt.Errorf("more then two OVNDBCluster object found in namespace %s", namespace)
	}

	if len(ovnDBList.Items) == 0 {
		return nil, k8s_errors.NewNotFound(
			appsv1.Resource("OVNDBCluster"),
			fmt.Sprintf("No OVNDBCluster object found in namespace %s", namespace),
		)
	}
	DBEndpointsMap := make(map[string]string)
	for _, ovndb := range ovnDBList.Items {
		if ovndb.Status.DBAddress != "" {
			DBEndpointsMap[ovndb.Spec.DBType] = ovndb.Status.DBAddress
		} else {
			return DBEndpointsMap, fmt.Errorf("DBEndpoint not ready yet for %s", ovndb.Spec.DBType)
		}
	}
	return DBEndpointsMap, nil
}

func getItems(list client.ObjectList) []client.Object {
	items := []client.Object{}
	values := reflect.ValueOf(list).Elem().FieldByName("Items")
	for i := 0; i < values.Len(); i++ {
		item := values.Index(i)
		if item.Kind() == reflect.Pointer {
			items = append(items, item.Interface().(client.Object))
		} else {
			items = append(items, item.Addr().Interface().(client.Object))
		}
	}

	return items
}

// OVNDBClusterNamespaceMapFunc - DBCluster Watch Function
func OVNDBClusterNamespaceMapFunc(crs client.ObjectList, reader client.Reader, log logr.Logger) handler.MapFunc {
	return func(obj client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all CRs from the same namespace, right now there should only be one
		listOpts := []client.ListOption{
			client.InNamespace(obj.GetNamespace()),
		}
		if err := reader.List(context.Background(), crs, listOpts...); err != nil {
			log.Error(err, "Unable to retrieve self CRs %v")
			return nil
		}
		for _, cr := range getItems(crs) {
			if obj.GetNamespace() == cr.GetNamespace() {
				// return namespace and Name of CR
				name := client.ObjectKey{
					Namespace: cr.GetNamespace(),
					Name:      cr.GetName(),
				}
				result = append(result, reconcile.Request{NamespacedName: name})
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}
}
