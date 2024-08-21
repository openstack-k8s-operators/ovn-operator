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

package ovncontroller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getPhysicalNetworks(
	instance *ovnv1.OVNController,
) string {
	// NOTE(slaweq): to make things easier, each physical bridge will have
	//               the same name as "br-<physical network>"
	// NOTE(slaweq): interface names aren't important as inside Pod they will be
	//               named based on the NicMappings keys
	// Need to pass sorted data as Map is unordered
	nicMappings := maps.Keys(instance.Spec.NicMappings)
	sort.Strings(nicMappings)
	return strings.Join(nicMappings, " ")
}

func getOVNControllerPods(
	ctx context.Context,
	k8sClient client.Client,
	instance *ovnv1.OVNController,
) (*corev1.PodList, error) {

	podList := &corev1.PodList{}
	podListOpts := &client.ListOptions{
		Namespace: instance.Namespace,
	}
	client.MatchingLabels{
		"service": ovnv1.ServiceNameOVNController,
	}.ApplyToList(podListOpts)

	if err := k8sClient.List(ctx, podList, podListOpts); err != nil {
		err = fmt.Errorf("error listing pods for instance %s: %w", instance.Name, err)
		return podList, err
	}

	return podList, nil
}
