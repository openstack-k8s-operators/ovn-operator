// Package common contains shared constants and utilities for the OVN operator
package common // nolint:revive

const (
	// OVNDbCertPath is the path to the OVN database certificate file
	OVNDbCertPath string = "/etc/pki/tls/certs/ovndb.crt"
	// OVNDbKeyPath is the path to the OVN database private key file
	OVNDbKeyPath string = "/etc/pki/tls/private/ovndb.key"
	// OVNDbCaCertPath is the path to the OVN database CA certificate file
	OVNDbCaCertPath string = "/etc/pki/tls/certs/ovndbca.crt"

	// OVNMetricsCertPath is the path to the metrincs certificate file
	OVNMetricsCertPath string = "/etc/pki/tls/certs/ovnmetrics.crt"
	// OVNMetricsKeyPath is the path to the metrics private key file
	OVNMetricsKeyPath string = "/etc/pki/tls/private/ovnmetrics.key"

	// MetricsPort is the port used for metrics
	MetricsPort int32 = 1981
)
