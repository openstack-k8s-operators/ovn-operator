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
	"fmt"
	"time"

	certmgrv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmgrmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/openstack-k8s-operators/lib-common/modules/certmanager"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// selfSignedIssuerName is the name of the self-signed issuer used to
	// bootstrap the OVN RBAC PKI CA.
	selfSignedIssuerName = "ovn-rbac-selfsigned-issuer"
	// rbacCAIssuerName is the name of the CA issuer that uses the generated
	// CA certificate to issue per-node ovn-controller certificates.
	rbacCAIssuerName = "ovn-rbac-ca-issuer"
)

// EnsureRbacPkiCA creates the cert-manager resources needed for OVN RBAC PKI:
// a self-signed Issuer, a CA Certificate, and a CA Issuer that can then be
// used to sign per-node ovn-controller certificates. This follows the same
// pattern as the openstack-operator's createRootCACertAndIssuer.
func EnsureRbacPkiCA(
	ctx context.Context,
	h *helper.Helper,
	namespace string,
	labels map[string]string,
) (ctrl.Result, error) {
	// Step 1: Create self-signed issuer to bootstrap the CA
	selfSignedIssuerReq := certmanager.SelfSignedIssuer(
		selfSignedIssuerName,
		namespace,
		labels,
	)
	selfSignedIssuer := certmanager.NewIssuer(selfSignedIssuerReq, time.Duration(5)*time.Second)
	ctrlResult, err := selfSignedIssuer.CreateOrPatch(ctx, h)
	if err != nil {
		return ctrlResult, fmt.Errorf("failed to create self-signed issuer for OVN RBAC PKI: %w", err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Step 2: Create CA certificate signed by the self-signed issuer
	caCertReq := certmanager.Cert(
		OVNRbacPkiCaSecret,
		namespace,
		labels,
		certmgrv1.CertificateSpec{
			IsCA:       true,
			CommonName: "OVN RBAC CA",
			SecretName: OVNRbacPkiCaSecret,
			PrivateKey: &certmgrv1.CertificatePrivateKey{
				Algorithm: certmgrv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmgrmetav1.ObjectReference{
				Name:  selfSignedIssuerReq.Name,
				Kind:  selfSignedIssuerReq.Kind,
				Group: selfSignedIssuerReq.GroupVersionKind().Group,
			},
			Duration: &metav1.Duration{
				Duration: time.Hour * 24 * 365 * 10, // 10 years
			},
		},
	)
	caCert := certmanager.NewCertificate(caCertReq, time.Duration(5)*time.Second)
	ctrlResult, _, err = caCert.CreateOrPatch(ctx, h, nil)
	if err != nil {
		return ctrlResult, fmt.Errorf("failed to create OVN RBAC PKI CA certificate: %w", err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Step 3: Create CA issuer that uses the generated CA certificate
	caIssuerReq := certmanager.CAIssuer(
		rbacCAIssuerName,
		namespace,
		labels,
		nil, // annotations
		OVNRbacPkiCaSecret,
	)
	caIssuer := certmanager.NewIssuer(caIssuerReq, time.Duration(5)*time.Second)
	ctrlResult, err = caIssuer.CreateOrPatch(ctx, h)
	if err != nil {
		return ctrlResult, fmt.Errorf("failed to create OVN RBAC PKI CA issuer: %w", err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	return ctrl.Result{}, nil
}
