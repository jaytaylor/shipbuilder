package core

// Load-balancer types.

// LBAppDyno is a component of LBApp.
type LBAppDyno struct {
	Host string
	Port int
}

// LBApp is a component of LBSpec.
type LBApp struct {
	Name                    string
	Domains                 []string
	FirstDomain             string
	Servers                 []*LBAppDyno
	Maintenance             bool
	MaintenancePageFullPath string
	MaintenancePageBasePath string
	MaintenancePageDomain   string
	SSL                     bool // Whether or not to enable SSL for the app.
	SSLForwarding           bool // Whether or not to enable automatic SSL redirection.
}

// LBSpec contains information required to feed the HAProxy template generator.
type LBSpec struct {
	LogServerIpAndPort  string // ShipBuilder server ip:port to send HAProxy UDP logs to.
	Applications        []*LBApp
	LoadBalancers       []string
	HaProxyStatsEnabled bool
	HaProxyCredentials  string
}

// SSLForwardingDomains returns the list of domain names which have SSL
// forwarding enabled.
func (lbSpec LBSpec) SSLForwardingDomains() []string {
	found := []string{}
	for _, app := range lbSpec.Applications {
		if app.SSL && app.SSLForwarding {
			found = append(found, app.Domains...)
		}
	}
	return found
}

// DynHdrFlags returns dynamic flags to include to the HAProxy `hdr(host)'
// function.  For example, when SB_ENABLE_NONSTANDARD_LB_PORTS is enabled, the
// "-m beg" flag is necessary to ensure reachability to apps via the LB.
func (lbSpec LBSpec) DynHdrFlags() string {
	if isTruthy(DefaultHAProxyEnableNonstandardPorts) {
		return "-m beg "
	}
	return ""
}
