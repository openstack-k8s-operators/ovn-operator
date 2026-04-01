// Package ovndbcluster provides functionality for managing OVN database clusters
package ovndbcluster

const (
	// DbPortNB is the port number for the OVN Northbound database
	DbPortNB int32 = 6641
	// RaftPortNB is the port number for the OVN Northbound database Raft protocol
	RaftPortNB int32 = 6643

	// DbPortSB is the port number for the OVN Southbound database
	DbPortSB int32 = 6642
	// DbPortSBRBACFullAccess is the port number for the OVN Southbound database
	// which provides connection with full access to the SB db.
	// In case when DbPortSB provides RBAC listener for ovn-controller,
	// this port is used to provide listener with full write access used by Northd.
	DbPortSBRBACFullAccess int32 = 16642
	// RaftPortSB is the port number for the OVN Southbound database Raft protocol
	RaftPortSB int32 = 6644

	// OVNRbacPkiCaSecret is the name of the K8s Secret that stores the OVN RBAC
	// PKI CA certificate and key, used for signing per-node ovn-controller
	// certificates when RBAC is enabled (TLS enabled on SB).
	OVNRbacPkiCaSecret = "ovn-rbac-pki-ca" //nolint:gosec
)
