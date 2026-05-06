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
	"time"

	certmgrv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmgrmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EnsureRbacCert creates or updates a cert-manager Certificate CR for a node's
// RBAC client certificate, and waits for the resulting Secret to become ready.
func EnsureRbacCert(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	instance *ovnv1.OVNController,
	certName string,
	commonName string,
	issuerName string,
	labels map[string]string,
) (ctrl.Result, error) {
	Log := log.FromContext(ctx).WithName("Controllers").WithName("OVNController")

	cert := &certmgrv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: certmgrv1.CertificateSpec{
			CommonName: commonName,
			SecretName: certName,
			IssuerRef: certmgrmetav1.ObjectReference{
				Name: issuerName,
				Kind: "Issuer",
			},
			Usages: []certmgrv1.KeyUsage{
				certmgrv1.UsageClientAuth,
				certmgrv1.UsageDigitalSignature,
			},
		},
	}

	existing := &certmgrv1.Certificate{}
	err := c.Get(ctx, types.NamespacedName{Name: certName, Namespace: instance.Namespace}, existing)
	if err != nil {
		if !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		err = controllerutil.SetControllerReference(instance, cert, scheme)
		if err != nil {
			return ctrl.Result{}, err
		}
		Log.Info("Creating RBAC certificate", "name", certName, "cn", commonName)
		err = c.Create(ctx, cert)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	if existing.Spec.CommonName != commonName || existing.Spec.IssuerRef.Name != issuerName {
		existing.Spec.CommonName = commonName
		existing.Spec.IssuerRef.Name = issuerName
		err = c.Update(ctx, existing)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	certSecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: certName, Namespace: instance.Namespace}, certSecret)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("Waiting for cert-manager to create secret", "name", certName)
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	if _, ok := certSecret.Data["tls.crt"]; !ok {
		Log.Info("Cert secret missing tls.crt, waiting", "name", certName)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}
	if _, ok := certSecret.Data["tls.key"]; !ok {
		Log.Info("Cert secret missing tls.key, waiting", "name", certName)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	return ctrl.Result{}, nil
}
