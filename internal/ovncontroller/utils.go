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

	"github.com/google/uuid"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ComputeSystemID derives a deterministic UUID5 system-id from a node name,
// matching the convention used by OVN for chassis identification.
func ComputeSystemID(nodeName string) string {
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(nodeName)).String()
}

// RbacCertName returns the cert-manager Certificate CR name for a given node
func RbacCertName(nodeName string) string {
	return fmt.Sprintf("ovn-controller-cert-%s", nodeName)
}

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

// GetOVNControllerPods returns list of the pods running ovn-controller
func GetOVNControllerPods(
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
