package pingutil

import "context"

// PingAuto2 is retained for compatibility. New callers should use PingAutoConfig.
func PingAuto2(target string, config *PingConfig) *PingResult {
	return PingAutoConfig(target, func(c *PingConfig) {
		if config != nil {
			*c = *config
		}
	})
}

// PcapxPing is retained for compatibility and uses the shared netstack ICMP client.
func PcapxPing(target string, config *PingConfig) (*PingResult, error) {
	if config == nil {
		config = NewPingConfig()
	}
	ctx := config.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	return NetstackPing(ctx, target, config.linkAddressResolveTimeout)
}
