// Package common contains shared constants and utilities for the OVN operator
package common

const (
	// OVNDbCertPath is the path to the OVN database certificate file
	OVNDbCertPath string = "/etc/pki/tls/certs/ovndb.crt"
	// OVNDbKeyPath is the path to the OVN database private key file
	OVNDbKeyPath string = "/etc/pki/tls/private/ovndb.key"
	// OVNDbCaCertPath is the path to the OVN database CA certificate file
	OVNDbCaCertPath string = "/etc/pki/tls/certs/ovndbca.crt"
)
