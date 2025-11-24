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
	"errors"
	"fmt"
	"slices"
	"unicode"

	netattdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"golang.org/x/exp/maps"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

var errBondLink = errors.New("bondWithNoLinksError")

func bondWithNoLinksError(interfaceName string) error {
	return fmt.Errorf("%w: bond configuration for %s has no links defined", errBondLink, interfaceName)
}

func defineNADConfig(
	physNet string,
	interfaceName string,
) string {
	return fmt.Sprintf(
		`{"cniVersion": "0.3.1", "name": "%s", "type": "host-device", "device": "%s"}`,
		physNet, interfaceName)
}

func defineBondNADConfig(
	physNet string,
	bond ovnv1.Bond,
	links []string,
) string {
	linkNames := `[`
	for i, link := range links {
		linkNames = linkNames + fmt.Sprintf(`{"name": "%s"}`, link)
		if i < len(links)-1 {
			linkNames = linkNames + ", "
		}
	}
	linkNames = linkNames + `]`

	return fmt.Sprintf(
		`{"cniVersion": "0.3.1","name": "%s", "type": "bond", "mode": "%s", "failOverMac": 1, "linksInContainer": true, "miimon": "100", "mtu": %d, "links": %s}`,
		physNet, bond.Mode, bond.Mtu, linkNames)
}

// Create Network Attachment Definitions for links of a bond
func createLinkNADs(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNController,
	labels map[string]string,
	links []string,
) ([]string, error) {

	var linkNames []string

	for _, interfaceName := range links {

		var nameRunes []rune

		// Replace unwanted characters with a hyphen to create interfaces for them
		for _, char := range interfaceName {
			if unicode.IsLetter(char) || unicode.IsNumber(char) || char == '-' {
				nameRunes = append(nameRunes, char)
			} else {
				nameRunes = append(nameRunes, '-')
			}
		}
		nadName := string(nameRunes)
		nadConfig := defineNADConfig(nadName, interfaceName)
		err := createOrUpdateNetworkAttachmentDefinition(ctx, h, instance, labels, nadName, nadConfig)
		if err != nil {
			return nil, err
		}
		linkNames = append(linkNames, nadName)
	}
	return linkNames, nil
}

func createOrUpdateNetworkAttachmentDefinition(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNController,
	labels map[string]string,
	physNet string,
	nadSpecConfig string,
) error {
	var nad *netattdefv1.NetworkAttachmentDefinition

	nadSpec := netattdefv1.NetworkAttachmentDefinitionSpec{
		Config: nadSpecConfig,
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
			return fmt.Errorf("cannot get NetworkAttachmentDefinition %s: %w", nadSpecConfig, err)
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
			return fmt.Errorf("cannot create NetworkAttachmentDefinition %s: %w", nadSpecConfig, err)
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
				return fmt.Errorf("cannot update NetworkAttachmentDefinition %s: %w", nadSpecConfig, err)
			}
		}
	}

	return nil
}

// CreateOrUpdateAdditionalNetworks - create or update network attachment definitions based on the provided mappings
func CreateOrUpdateAdditionalNetworks(
	ctx context.Context,
	h *helper.Helper,
	instance *ovnv1.OVNController,
	labels map[string]string,
	networkAttachments []string,
) ([]string, error) {

	var finalNetworkAttachments []string
	bondKeys := maps.Keys(instance.Spec.BondConfiguration)

	// Sort the NIC keys to ensure they are added in a consistent order in the pod configuration.
	// Links need to be added before the bond NAD, and nicmappings need to be added in alphabetical order.
	nicKeys := maps.Keys(instance.Spec.NicMappings)
	allKeys := append(nicKeys, networkAttachments...)
	slices.Sort(allKeys)

	for _, physNet := range allKeys {
		// Don't process network attachments that were already in the networkAttachments list
		if slices.Contains(networkAttachments, physNet) {
			finalNetworkAttachments = append(finalNetworkAttachments, physNet)
			continue
		}
		// If the name of the interface is configured as bond, add bond NAD instead of the default nic NAD.
		interfaceName := instance.Spec.NicMappings[physNet]
		var nadConfig string
		if slices.Contains(bondKeys, interfaceName) {
			if len(instance.Spec.BondConfiguration[interfaceName].Links) == 0 {
				return nil, bondWithNoLinksError(interfaceName)
			}
			linkNames, err := createLinkNADs(ctx, h, instance, labels, instance.Spec.BondConfiguration[interfaceName].Links)
			if err != nil {
				return nil, err
			}
			finalNetworkAttachments = append(finalNetworkAttachments, linkNames...)
			nadConfig = defineBondNADConfig(physNet, instance.Spec.BondConfiguration[interfaceName], linkNames)
		} else {
			nadConfig = defineNADConfig(physNet, interfaceName)
		}
		err := createOrUpdateNetworkAttachmentDefinition(ctx, h, instance, labels, physNet, nadConfig)
		if err != nil {
			return nil, err
		}
		finalNetworkAttachments = append(finalNetworkAttachments, physNet)

	}
	return finalNetworkAttachments, nil
}
