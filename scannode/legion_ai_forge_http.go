package scannode

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type legionForgeHTTPRequestEvidence struct {
	Method     string
	StatusCode int
	BodySHA256 string
}

type legionForgeLookupIPFunc func(context.Context, string) ([]net.IP, error)

func defaultLegionForgeLookupIP(ctx context.Context, host string) ([]net.IP, error) {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	result := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, append(net.IP(nil), address.IP...))
	}
	return result, nil
}

func newLegionForgeHTTPRuntime(
	ctx context.Context,
	target string,
	lookup legionForgeLookupIPFunc,
) (*legionServerFocusRuntime, error) {
	authorized, err := normalizeServerFocusURL(target)
	if err != nil {
		return nil, err
	}
	client, err := newLegionForgeHTTPClient(ctx, authorized, lookup)
	if err != nil {
		return nil, err
	}
	return &legionServerFocusRuntime{ctx: ctx, authorized: authorized, client: client}, nil
}

// newLegionForgeHTTPClient resolves the authorized origin once and pins all
// dials to that reviewed address set. DNS names may resolve only to public
// addresses; RFC1918/ULA targets must be explicit IP literals. This keeps
// rebinding, loopback, link-local and platform metadata endpoints outside the
// model-controlled request surface while retaining deliberate intranet tests.
func newLegionForgeHTTPClient(
	ctx context.Context,
	authorized *url.URL,
	lookup legionForgeLookupIPFunc,
) (*http.Client, error) {
	if authorized == nil || lookup == nil {
		return nil, fmt.Errorf("authorized HTTP origin is unavailable")
	}
	host := strings.ToLower(strings.TrimSpace(authorized.Hostname()))
	if blockedLegionForgeHostname(host) {
		return nil, fmt.Errorf("container-local or platform HTTP target is not allowed")
	}
	literal := net.ParseIP(host)
	var addresses []net.IP
	if literal != nil {
		addresses = []net.IP{literal}
	} else {
		resolved, err := lookup(ctx, host)
		if err != nil || len(resolved) == 0 {
			return nil, fmt.Errorf("resolve authorized HTTP target: %w", err)
		}
		addresses = resolved
	}
	pinned := make([]net.IP, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		address = normalizedLegionForgeIP(address)
		if address == nil || blockedLegionForgeIP(address) || (literal == nil && address.IsPrivate()) {
			return nil, fmt.Errorf("resolved HTTP target address is not allowed")
		}
		key := address.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		pinned = append(pinned, address)
	}
	if len(pinned) == 0 {
		return nil, fmt.Errorf("authorized HTTP target has no usable address")
	}

	dialer := &net.Dialer{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		dialHost, dialPort, err := net.SplitHostPort(address)
		if err != nil || !strings.EqualFold(strings.Trim(dialHost, "[]"), host) || dialPort != serverFocusEffectivePort(authorized) {
			return nil, fmt.Errorf("HTTP dial escaped the authorized origin")
		}
		var lastErr error
		for _, pinnedIP := range pinned {
			connection, dialErr := dialer.DialContext(dialCtx, network, net.JoinHostPort(pinnedIP.String(), dialPort))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
	return &http.Client{
		Transport: transport,
		Timeout:   serverFocusRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func normalizedLegionForgeIP(address net.IP) net.IP {
	if ipv4 := address.To4(); ipv4 != nil {
		return append(net.IP(nil), ipv4...)
	}
	if ipv6 := address.To16(); ipv6 != nil {
		return append(net.IP(nil), ipv6...)
	}
	return nil
}

func blockedLegionForgeIP(address net.IP) bool {
	if !address.IsGlobalUnicast() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() {
		return true
	}
	for _, raw := range []string{"100.100.100.100", "100.100.100.200", "fd00:ec2::254"} {
		if address.Equal(net.ParseIP(raw)) {
			return true
		}
	}
	return false
}

func blockedLegionForgeHostname(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") ||
		host == "host.docker.internal" || host == "gateway.docker.internal" ||
		host == "metadata.google.internal" || host == "kubernetes.default" ||
		strings.HasSuffix(host, ".svc") || strings.HasSuffix(host, ".cluster.local") {
		return true
	}
	return false
}

func (r *legionServerFocusRuntime) applicationHTTPMaterialReferences() []aiApplicationMaterialReference {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	evidence := append([]legionForgeHTTPRequestEvidence(nil), r.httpEvidence...)
	r.mu.Unlock()
	result := make([]aiApplicationMaterialReference, 0, len(evidence))
	for index, item := range evidence {
		identity := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%d\x00%s", index, item.Method, item.StatusCode, item.BodySHA256)))
		result = append(result, aiApplicationMaterialReference{
			Kind: "http_request", InputKey: "target-url",
			ResourceID: fmt.Sprintf("http_%x", identity[:16]), SHA256: item.BodySHA256,
			Operations: []string{strings.ToLower(item.Method), fmt.Sprintf("status:%d", item.StatusCode)},
		})
	}
	return result
}
