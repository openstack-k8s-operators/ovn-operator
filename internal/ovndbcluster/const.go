// Package ovndbcluster provides functionality for managing OVN database clusters
package ovndbcluster

const (
	// DbPortNB is the port number for the OVN Northbound database
	DbPortNB int32 = 6641
	// RaftPortNB is the port number for the OVN Northbound database Raft protocol
	RaftPortNB int32 = 6643

	// DbPortSB is the port number for the OVN Southbound database
	DbPortSB int32 = 6642
	// RaftPortSB is the port number for the OVN Southbound database Raft protocol
	RaftPortSB int32 = 6644
)
