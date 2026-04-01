package ovncontroller

const (
	// OVNControllerCertDir is the directory where per-node ovn-controller
	// RBAC certificates are stored (HostPath-backed)
	OVNControllerCertDir string = "/etc/openvswitch"
	// OVNControllerCertPath is the path to the per-node ovn-controller certificate
	OVNControllerCertPath string = OVNControllerCertDir + "/ovn-controller-cert.pem"
	// OVNControllerKeyPath is the path to the per-node ovn-controller private key
	OVNControllerKeyPath string = OVNControllerCertDir + "/ovn-controller-privkey.pem"
)
