package common

const (
	OVNDbCertPath   string = "/etc/pki/tls/certs/ovndb.crt"
	OVNDbKeyPath    string = "/etc/pki/tls/private/ovndb.key"
	OVNDbCaCertPath string = "/etc/pki/tls/certs/ovndbca.crt"

	// Metrics certificate paths
	OVNMetricsCertPath string = "/etc/pki/tls/certs/ovnmetrics.crt"
	OVNMetricsKeyPath  string = "/etc/pki/tls/private/ovnmetrics.key"

	// Metrics port
	MetricsPort int32 = 1981
)
