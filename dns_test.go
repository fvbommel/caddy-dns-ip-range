package dns

import (
	"context"
	"testing"

	"github.com/caddyserver/caddy/v2"
)

func TestLookup(t *testing.T) {
	r := DNSRange{
		Hosts: []string{"one.one.one.one"},
	}

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	err := r.Provision(ctx)
	if err != nil {
		t.Errorf("error provisioning: %v", err)
	}

	ips := r.GetIPRanges(nil)

	if len(ips) == 0 {
		t.Errorf("no results")
	}

	for _, ip := range ips {
		if !ip.IsSingleIP() {
			t.Errorf("returned prefix too large")
		}
	}
}
