/*

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

package ovndbcluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_labels "k8s.io/apimachinery/pkg/labels"

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

// OVNDBPods - Query current running ovn db pods managed by the statefulset
func OVNDBPods(
	ctx context.Context,
	instance *ovnv1.OVNDBCluster,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (*corev1.PodList, error) {
	podSelectorString := k8s_labels.Set(serviceLabels).String()
	return helper.GetKClient().CoreV1().Pods(instance.Namespace).List(ctx, metav1.ListOptions{LabelSelector: podSelectorString})
}
