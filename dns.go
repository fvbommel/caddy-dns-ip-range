package dns

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

const (
	DefaultInterval = caddy.Duration(time.Minute)
)

func init() {
	caddy.RegisterModule(new(DNSRange))
}

// DNSRange provides a range of IP addresses associated with a DNS name.
// Each range will only contain a single IP.
type DNSRange struct {
	// A list of DNS names to look up.
	Hosts []string `json:"hosts,omitempty"`

	// The refresh interval. Defaults to DefaultInterval.
	Interval caddy.Duration `json:"interval,omitempty"`

	// After provisioning, access to the addresses map is guarded by this mutex.
	mu sync.RWMutex

	// Most recent resolved addresses of the configured hosts, stuffed into single-IP prefixes.
	addresses map[string][]netip.Prefix

	// Canceled when the module is being cleaned up.
	ctx caddy.Context

	// The logger.
	logger *zap.Logger
}

// CaddyModule returns the Caddy module information.
func (d *DNSRange) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.ip_sources.dns",
		New: func() caddy.Module { return new(DNSRange) },
	}
}

func (d *DNSRange) Provision(ctx caddy.Context) error {
	d.logger = ctx.Logger()

	// Sanity checks.
	if len(d.Hosts) == 0 {
		return errors.New("dns ip range: no host names provided")
	}

	if d.Interval < 0 {
		return errors.New("interval cannot be negative")
	}

	// Set defaults.
	if d.Interval == 0 {
		d.Interval = DefaultInterval
	}

	// Initialize internal fields.
	d.addresses = make(map[string][]netip.Prefix)
	d.ctx = ctx

	// Perform initial lookups.
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, host := range d.Hosts {
		// Look up initial IPs and store them as prefixes
		addresses, err := d.initialLookup(host)
		if err != nil {
			return fmt.Errorf("error looking up DNS name %q: %w", host, err)
		}

		d.addresses[host] = addresses
	}

	return nil
}

func (d *DNSRange) GetIPRanges(_ *http.Request) (result []netip.Prefix) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, addrs := range d.addresses {
		result = append(result, addrs...)
	}

	return result
}

func (d *DNSRange) initialLookup(host string) ([]netip.Prefix, error) {
	prefixes, err := d.lookupHostPrefixes(host)

	// If we're successful, keep this host updated.
	if err == nil {
		go d.keepUpdated(host)
	}

	return prefixes, err
}

func (d *DNSRange) keepUpdated(host string) {
	const ttlAfterErr = time.Minute

	d.logger.Info("starting DNS watcher", zap.String("host", host))

	done := d.ctx.Done()
	freq := time.Duration(d.Interval)
	ticker := time.NewTicker(freq)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			d.logger.Info("stopping DNS watcher", zap.String("host", host))
			return
		case <-ticker.C:
			// fall through
		}

		// Look up host.
		prefixes, err := d.lookupHostPrefixes(host)
		newFreq := time.Duration(d.Interval)
		if err == nil {
			d.mu.Lock()
			d.addresses[host] = prefixes
			d.mu.Unlock()
		} else {
			// TODO: Inspect error. Treat NXDOMAIN as empty result?

			// Log unhandled error
			d.logger.Warn("DNS lookup error",
				zap.String("host", host),
				zap.Error(err))

			// Check again after a while.
			// TODO: Exponential backoff?
			newFreq = ttlAfterErr
		}

		// Has the update frequency changed?
		if newFreq != freq {
			ticker.Reset(newFreq)
			freq = newFreq
		}
	}
}

func (d *DNSRange) lookupHostPrefixes(host string) (prefixes []netip.Prefix, err error) {
	ips, err := net.LookupHost(host)
	if err != nil {
		d.logger.Warn("DNS error", zap.Error(err))
		return nil, err
	}

	prefixes = make([]netip.Prefix, 0, len(ips))

	for _, ip := range ips {
		addr, err := netip.ParseAddr(ip)
		if err == nil {
			prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
		} else {
			d.logger.Warn("ignoring invalid IP address", zap.String("ip", ip), zap.Error(err))
		}
	}

	if len(prefixes) == 0 && cap(prefixes) != 0 {
		return nil, errors.New("all returned IP addresses were invalid")
	}

	d.logger.Debug("DNS results",
		zap.String("host", host),
		zap.Strings("addresses", ips))

	return prefixes, nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
//
// Example config, if you're running cloudflared on the same Docker bridge network as Caddy:
//
//	trusted_proxies dns cloudflared {
//	    # Explicitly set the default value.
//	    interval 1m
//	}
//
// Alternative syntax:
//
//	trusted_proxies dns {
//	    host cloudflared
//	    # Explicitly set the default value.
//	    interval 1m
//	}
//
// Multiple host names are supported, all on the same line and/or
// in multiple host directives.
func (m *DNSRange) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	if !d.Next() {
		return nil
	}

	// Inline hosts
	m.Hosts = d.RemainingArgs()

	for nesting := d.Nesting(); d.NextBlock(nesting); {
		switch d.Val() {
		case "host":
			args := d.RemainingArgs()
			if len(args) == 0 {
				return d.ArgErr()
			}
			m.Hosts = append(m.Hosts, args...)

		case "interval":
			if !d.NextArg() {
				return d.Err("expected duration")
			}
			interval, err := caddy.ParseDuration(d.Val())
			if err != nil {
				return d.WrapErr(err)
			}
			m.Interval = caddy.Duration(interval)
		}
		// TODO: some way of specifying error handling for network errors/NXDOMAIN?
	}

	return nil
}

// Interface guards
var (
	_ caddy.Module            = (*DNSRange)(nil)
	_ caddy.Provisioner       = (*DNSRange)(nil)
	_ caddyfile.Unmarshaler   = (*DNSRange)(nil)
	_ caddyhttp.IPRangeSource = (*DNSRange)(nil)
)
