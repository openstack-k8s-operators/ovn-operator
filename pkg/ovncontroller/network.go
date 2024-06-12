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

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

// CreateOrUpdateAdditionalNetworks - create or update network attachment definitions based on the provided mappings
func CreateOrUpdateAdditionalNetworks(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNController,
	labels map[string]string,
) ([]string, error) {

	var nad *netattdefv1.NetworkAttachmentDefinition
	var networkAttachments []string

	for physNet, interfaceName := range instance.Spec.NicMappings {
		nadSpec := netattdefv1.NetworkAttachmentDefinitionSpec{
			Config: fmt.Sprintf(
				`{"cniVersion": "0.3.1", "name": "%s", "type": "host-device", "device": "%s"}`,
				physNet, interfaceName),
		}
		nad = &netattdefv1.NetworkAttachmentDefinition{}
		err := h.GetClient().Get(
			ctx,
			client.ObjectKey{
				Namespace: instance.Namespace,
				Name:      physNet,
			},
			nad,
		)
		if err != nil {
			if !k8s_errors.IsNotFound(err) {
				return nil, fmt.Errorf("cannot get NetworkAttachmentDefinition %s/%s: %w",
					physNet, interfaceName, err)
			}

			ownerRef := metav1.NewControllerRef(instance, instance.GroupVersionKind())
			nad = &netattdefv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:            physNet,
					Namespace:       instance.Namespace,
					Labels:          labels,
					OwnerReferences: []metav1.OwnerReference{*ownerRef},
				},
				Spec: nadSpec,
			}
			// Request object not found, lets create it
			if err := h.GetClient().Create(ctx, nad); err != nil {
				return nil, fmt.Errorf("cannot create NetworkAttachmentDefinition %s/%s: %w",
					physNet, interfaceName, err)
			}
		} else {
			owned := false
			for _, owner := range nad.GetOwnerReferences() {
				if owner.Name == instance.Name {
					owned = true
					break
				}
			}
			if owned {
				nad.Spec = nadSpec
				if err := h.GetClient().Update(ctx, nad); err != nil {
					return nil, fmt.Errorf("cannot update NetworkAttachmentDefinition %s/%s: %w",
						physNet, interfaceName, err)
				}
			}
		}

		networkAttachments = append(networkAttachments, physNet)
	}

	return networkAttachments, nil
}
