package ovndbcluster

import (
	"context"
	"fmt"

	infranetworkv1 "github.com/openstack-k8s-operators/infra-operator/apis/network/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DNSData - Create DNS entry that openstack dnsmasq will resolve
func DNSData(
	ctx context.Context,
	helper *helper.Helper,
	serviceName string,
	ipList []string,
	instance *ovnv1.OVNDBCluster,
	serviceLabels map[string]string,
) error {
	// ovsdbserver-(sb|nb) entry
	headlessDNSHostname := serviceName + "." + instance.Namespace + ".svc"
	dnsHosts := []infranetworkv1.DNSHost{}
	for _, ip := range ipList {
		record := infranetworkv1.DNSHost{
			IP: ip,
			Hostnames: []string{
				headlessDNSHostname,
			},
		}
		dnsHosts = append(dnsHosts, record)

	}

	// Create DNSData object
	dnsData := &infranetworkv1.DNSData{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, helper.GetClient(), dnsData, func() error {
		dnsData.Spec.Hosts = dnsHosts
		// TODO: use value from DNSMasq instance instead of hardcode
		dnsData.Spec.DNSDataLabelSelectorValue = "dnsdata"
		err := controllerutil.SetControllerReference(helper.GetBeforeObject(), dnsData, helper.GetScheme())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("Error creating DNSData %s: %w", dnsData.Name, err)
	}
	return nil
}

// GetDBAddress - return string connection for the given service
func GetDBAddress(svcSpec *corev1.ServiceSpec, serviceName string, namespace string, scheme string) string {
	if svcSpec == nil {
		return ""
	}
	headlessDNSHostname := serviceName + "." + namespace + ".svc"
	return fmt.Sprintf("%s:%s:%d", scheme, headlessDNSHostname, svcSpec.Ports[0].Port)
}
