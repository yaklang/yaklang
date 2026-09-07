package minimartian

import (
	"context"
	"net"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type upstreamDialResult struct {
	conn net.Conn
	err  error
}

// upstreamTransportConfig applies the same transport-level lowhttp options as
// ordinary MITM upstream requests. HTTP/TLS-only options may exist on the
// returned config, but raw tunnels intentionally consume only TCP dial fields.
func (p *Proxy) upstreamTransportConfig() *lowhttp.LowhttpExecConfig {
	config := lowhttp.NewLowhttpOption()
	if p == nil {
		return config
	}
	for _, apply := range p.lowhttpConfig {
		if apply != nil {
			apply(config)
		}
	}
	// execLowhttp applies Proxy.SetDialer after the shared lowhttp options, so
	// preserve the same precedence here.
	if p.dialer != nil {
		config.Dialer = p.dialer
	}
	// Disabling the system proxy is an explicit MITM override. When it is not
	// disabled, leave the setting untouched so netx's global configuration wins.
	if p.disableSystemProxy {
		config.OverrideEnableSystemProxyFromEnv = true
		config.EnableSystemProxyFromEnv = false
	}
	return config
}

func dialUpstreamWithContext(ctx context.Context, target string, opts ...netx.DialXOption) (net.Conn, error) {
	if ctx == nil || ctx.Done() == nil {
		return netx.DialX(target, opts...)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resultCh := make(chan upstreamDialResult)
	go func() {
		conn, err := netx.DialX(target, opts...)
		result := upstreamDialResult{conn: conn, err: err}
		select {
		case resultCh <- result:
		case <-ctx.Done():
			if conn != nil {
				_ = conn.Close()
			}
		}
	}()

	select {
	case result := <-resultCh:
		if err := ctx.Err(); err != nil {
			if result.conn != nil {
				_ = result.conn.Close()
			}
			return nil, err
		}
		return result.conn, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *Proxy) dialRawTunnel(ctx context.Context, target, host, strongHostLocalAddr string) (net.Conn, error) {
	config := p.upstreamTransportConfig()
	proxies := p.selectProxiesForHost(host)
	bypassProxy := p.isDownstreamProxyBypassedForHost(host)
	if len(proxies) == 0 && !bypassProxy {
		proxies = append([]string(nil), config.Proxy...)
	}

	dialTarget := target
	if len(proxies) > 0 && config.PreferEtcHostsBeforeProxy {
		if mappedHost, ok := netx.ResolveHostByTemporaryHosts(host, config.EtcHosts); ok {
			_, port, err := utils.ParseStringToHostPort(target)
			if err != nil {
				return nil, utils.Wrapf(err, "parse raw tunnel target %s", target)
			}
			dialTarget = utils.HostPort(mappedHost, port)
		}
	}

	dnsOpts := []netx.DNSOption{
		netx.WithDNSServers(config.DNSServers...),
		netx.WithTemporaryHosts(config.EtcHosts),
	}
	if ctx != nil {
		dnsOpts = append(dnsOpts, netx.WithDNSContext(ctx))
	}
	opts := []netx.DialXOption{
		netx.DialX_WithTimeout(config.ConnectTimeout),
		netx.DialX_WithTimeoutRetry(config.RetryTimes),
		netx.DialX_WithTimeoutRetryWaitRange(config.RetryWaitTime, config.RetryMaxWaitTime),
		netx.DialX_WithDNSOptions(dnsOpts...),
	}
	if len(proxies) > 0 {
		opts = append(opts,
			netx.DialX_WithProxy(proxies...),
			netx.DialX_WithForceProxy(true),
		)
	}
	if config.Dialer != nil {
		opts = append(opts, netx.DialX_WithDialer(config.Dialer))
	}
	if config.OverrideEnableSystemProxyFromEnv {
		opts = append(opts, netx.DialX_WithEnableSystemProxyFromEnv(config.EnableSystemProxyFromEnv))
	}
	if strongHostLocalAddr == "" {
		strongHostLocalAddr = config.StrongHost
	}
	if strongHostLocalAddr != "" {
		opts = append(opts, netx.DialX_WithStrongHostMode(strongHostLocalAddr))
	}
	if len(config.ExtendDialOption) > 0 {
		opts = append(opts, config.ExtendDialOption...)
	}
	return dialUpstreamWithContext(ctx, dialTarget, opts...)
}

// handleRawTunnel applies tcpmitm's default behavior for an unknown protocol:
// preserve the sniffed bytes and transparently forward the TCP stream.
func (p *Proxy) handleRawTunnel(ctx context.Context, downstream net.Conn, host string, port int, strongHostLocalAddr string) error {
	target := utils.HostPort(host, port)
	upstream, err := p.dialRawTunnel(ctx, target, host, strongHostLocalAddr)
	if err != nil {
		return utils.Errorf("dial raw tunnel target %s failed: %s", target, err)
	}
	defer upstream.Close()

	return connectionFallback(ctx, downstream, upstream, false)
}
