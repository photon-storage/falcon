package node

import (
	"testing"

	"github.com/ipfs/boxo/gateway"
	"github.com/stretchr/testify/assert"
)

func TestPortStripping(t *testing.T) {
	for _, test := range []struct {
		in  string
		out string
	}{
		{"localhost:8080", "localhost"},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.localhost:8080", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.localhost"},
		{"example.com:443", "example.com"},
		{"example.com", "example.com"},
		{"foo-dweb.ipfs.pvt.k12.ma.us:8080", "foo-dweb.ipfs.pvt.k12.ma.us"},
		{"localhost", "localhost"},
		{"[::1]:8080", "::1"},
	} {
		t.Run(test.in, func(t *testing.T) {
			out := stripPort(test.in)
			assert.Equal(t, test.out, out)
		})
	}
}

func TestKnownSubdomainDetails(t *testing.T) {
	gwLocalhost := &gateway.PublicGateway{
		Paths:         []string{"/ipfs", "/ipns", "/api"},
		UseSubdomains: true,
	}
	gwDweb := &gateway.PublicGateway{
		Paths:         []string{"/ipfs", "/ipns", "/api"},
		UseSubdomains: true,
	}
	gwLong := &gateway.PublicGateway{
		Paths:         []string{"/ipfs", "/ipns", "/api"},
		UseSubdomains: true,
	}
	gwWildcard1 := &gateway.PublicGateway{
		Paths:         []string{"/ipfs", "/ipns", "/api"},
		UseSubdomains: true,
	}
	gwWildcard2 := &gateway.PublicGateway{
		Paths:         []string{"/ipfs", "/ipns", "/api"},
		UseSubdomains: true,
	}

	gateways := prepareHostnameGateways(map[string]*gateway.PublicGateway{
		"localhost":               gwLocalhost,
		"dweb.link":               gwDweb,
		"devgateway.dweb.link":    gwDweb,
		"dweb.ipfs.pvt.k12.ma.us": gwLong, // note the sneaky ".ipfs." ;-)
		"*.wildcard1.tld":         gwWildcard1,
		"*.*.wildcard2.tld":       gwWildcard2,
	})

	for _, test := range []struct {
		// in:
		hostHeader string
		// out:
		gw       *gateway.PublicGateway
		hostname string
		ns       string
		rootID   string
		ok       bool
	}{
		// no subdomain
		{"127.0.0.1:8080", nil, "", "", "", false},
		{"[::1]:8080", nil, "", "", "", false},
		{"hey.look.example.com", nil, "", "", "", false},
		{"dweb.link", nil, "", "", "", false},
		// malformed Host header
		{".....dweb.link", nil, "", "", "", false},
		{"link", nil, "", "", "", false},
		{"8080:dweb.link", nil, "", "", "", false},
		{" ", nil, "", "", "", false},
		{"", nil, "", "", "", false},
		// unknown gateway host
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.unknown.example.com", nil, "", "", "", false},
		// cid in subdomain, known gateway
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.localhost:8080", gwLocalhost, "localhost:8080", "ipfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.dweb.link", gwDweb, "dweb.link", "ipfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.devgateway.dweb.link", gwDweb, "devgateway.dweb.link", "ipfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		// capture everything before .ipfs.
		{"foo.bar.boo-buzz.ipfs.dweb.link", gwDweb, "dweb.link", "ipfs", "foo.bar.boo-buzz", true},
		// ipns
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.ipns.localhost:8080", gwLocalhost, "localhost:8080", "ipns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.ipns.dweb.link", gwDweb, "dweb.link", "ipns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		// edge case check: public gateway under long TLD (see: https://publicsuffix.org)
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.dweb.ipfs.pvt.k12.ma.us", gwLong, "dweb.ipfs.pvt.k12.ma.us", "ipfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.ipns.dweb.ipfs.pvt.k12.ma.us", gwLong, "dweb.ipfs.pvt.k12.ma.us", "ipns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		// dnslink in subdomain
		{"en.wikipedia-on-ipfs.org.ipns.localhost:8080", gwLocalhost, "localhost:8080", "ipns", "en.wikipedia-on-ipfs.org", true},
		{"en.wikipedia-on-ipfs.org.ipns.localhost", gwLocalhost, "localhost", "ipns", "en.wikipedia-on-ipfs.org", true},
		{"dist.ipfs.tech.ipns.localhost:8080", gwLocalhost, "localhost:8080", "ipns", "dist.ipfs.tech", true},
		{"en.wikipedia-on-ipfs.org.ipns.dweb.link", gwDweb, "dweb.link", "ipns", "en.wikipedia-on-ipfs.org", true},
		// edge case check: public gateway under long TLD (see: https://publicsuffix.org)
		{"foo.dweb.ipfs.pvt.k12.ma.us", nil, "", "", "", false},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.dweb.ipfs.pvt.k12.ma.us", gwLong, "dweb.ipfs.pvt.k12.ma.us", "ipfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju.ipns.dweb.ipfs.pvt.k12.ma.us", gwLong, "dweb.ipfs.pvt.k12.ma.us", "ipns", "bafzbeihe35nmjqar22thmxsnlsgxppd66pseq6tscs4mo25y55juhh6bju", true},
		// other namespaces
		{"api.localhost", nil, "", "", "", false},
		{"peerid.p2p.localhost", gwLocalhost, "localhost", "p2p", "peerid", true},
		// wildcards
		{"wildcard1.tld", nil, "", "", "", false},
		{".wildcard1.tld", nil, "", "", "", false},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.wildcard1.tld", nil, "", "", "", false},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.sub.wildcard1.tld", gwWildcard1, "sub.wildcard1.tld", "ipfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.sub1.sub2.wildcard1.tld", nil, "", "", "", false},
		{"bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am.ipfs.sub1.sub2.wildcard2.tld", gwWildcard2, "sub1.sub2.wildcard2.tld", "ipfs", "bafkreicysg23kiwv34eg2d7qweipxwosdo2py4ldv42nbauguluen5v6am", true},
	} {
		t.Run(test.hostHeader, func(t *testing.T) {
			gw, hostname, ns, rootID, ok := gateways.knownSubdomainDetails(test.hostHeader)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.rootID, rootID)
			assert.Equal(t, test.ns, ns)
			assert.Equal(t, test.hostname, hostname)
			assert.Equal(t, test.gw, gw)
		})
	}
}
