package node

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	ipfsgw "github.com/ipfs/go-libipfs/gateway"
	"github.com/photon-storage/go-common/log"
)

// Copied from boxo/gateway/hostname.go
type hostnameGateways struct {
	exact    map[string]*ipfsgw.Specification
	wildcard map[*regexp.Regexp]*ipfsgw.Specification
}

// prepareHostnameGateways converts the user given gateways into an internal format
// split between exact and wildcard-based gateway hostnames.
func prepareHostnameGateways(gateways map[string]*ipfsgw.Specification) *hostnameGateways {
	h := &hostnameGateways{
		exact:    map[string]*ipfsgw.Specification{},
		wildcard: map[*regexp.Regexp]*ipfsgw.Specification{},
	}

	for hostname, gw := range gateways {
		if strings.Contains(hostname, "*") {
			// from *.domain.tld, construct a regexp that match any direct subdomain
			// of .domain.tld.
			//
			// Regexp will be in the form of ^[^.]+\.domain.tld(?::\d+)?$
			escaped := strings.ReplaceAll(hostname, ".", `\.`)
			regexed := strings.ReplaceAll(escaped, "*", "[^.]+")

			re, err := regexp.Compile(fmt.Sprintf(`^%s(?::\d+)?$`, regexed))
			if err != nil {
				log.Warn("invalid wildcard gateway hostname \"%s\"", hostname)
			}

			h.wildcard[re] = gw
		} else {
			h.exact[hostname] = gw
		}
	}

	return h
}

// isKnownHostname checks the given hostname gateways and returns a matching
// specification with graceful fallback to version without port.
func (gws *hostnameGateways) isKnownHostname(hostname string) (gw *ipfsgw.Specification, ok bool) {
	// Try hostname (host+optional port - value from Host header as-is)
	if gw, ok := gws.exact[hostname]; ok {
		return gw, ok
	}
	// Also test without port
	if gw, ok = gws.exact[stripPort(hostname)]; ok {
		return gw, ok
	}

	// Wildcard support. Test both with and without port.
	for re, spec := range gws.wildcard {
		if re.MatchString(hostname) {
			return spec, true
		}
	}

	return nil, false
}

// knownSubdomainDetails parses the Host header and looks for a known gateway matching
// the subdomain host. If found, returns a Specification and the subdomain components
// extracted from Host header: {rootID}.{ns}.{gwHostname}.
// Note: hostname is host + optional port
func (gws *hostnameGateways) knownSubdomainDetails(hostname string) (gw *ipfsgw.Specification, gwHostname, ns, rootID string, ok bool) {
	labels := strings.Split(hostname, ".")
	// Look for FQDN of a known gateway hostname.
	// Example: given "dist.ipfs.tech.ipns.dweb.link":
	// 1. Lookup "link" TLD in knownGateways: negative
	// 2. Lookup "dweb.link" in knownGateways: positive
	//
	// Stops when we have 2 or fewer labels left as we need at least a
	// rootId and a namespace.
	for i := len(labels) - 1; i >= 2; i-- {
		fqdn := strings.Join(labels[i:], ".")
		gw, ok := gws.isKnownHostname(fqdn)
		if !ok {
			continue
		}

		ns := labels[i-1]
		if !isSubdomainNamespace(ns) {
			continue
		}

		// Merge remaining labels (could be a FQDN with DNSLink)
		rootID := strings.Join(labels[:i-1], ".")
		return gw, fqdn, ns, rootID, true
	}
	// no match
	return nil, "", "", "", false
}

func stripPort(hostname string) string {
	host, _, err := net.SplitHostPort(hostname)
	if err == nil {
		return host
	}
	return hostname
}

func isSubdomainNamespace(ns string) bool {
	switch ns {
	case "ipfs", "ipns", "p2p", "ipld":
		// Note: 'p2p' and 'ipld' is only kept here for compatibility with Kubo.
		return true
	default:
		return false
	}
}
