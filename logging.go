package dns

import (
	"net/netip"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// logAddresses constructs a zap.Field of IP addresses from an array of prefixes.
func logAddresses(key string, ips []netip.Prefix) zap.Field {
	return zap.Array(key, ipArray(ips))
}

type ipArray []netip.Prefix

func (ss ipArray) MarshalLogArray(arr zapcore.ArrayEncoder) error {
	for i := range ss {
		arr.AppendString(ss[i].Addr().String())
	}
	return nil
}
