package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const legionDiscoveryBudget = 512

type legionForgeDiscoveryRuntime struct {
	ctx           context.Context
	target        string
	ports         []int
	addresses     []net.IP
	allowedLabels map[string]struct{}
	lookup        legionForgeLookupIPFunc
	dial          func(context.Context, string, string) (net.Conn, error)
	mu            sync.Mutex
	remaining     int
	calls         int
	evidence      []aiApplicationMaterialReference
}

func legionForgeDiscoveryParameters(release *aiv1.ContextForgeRelease) (string, []int, error) {
	if len(release.GetParameters()) > 16 {
		return "", nil, fmt.Errorf("discovery input field limit exceeded")
	}
	if _, err := legionForgeDiscoveryLabels(release); err != nil {
		return "", nil, err
	}
	target, ports := "", "80,443"
	for _, p := range release.GetParameters() {
		if p.GetKey() == "target-host" || p.GetKey() == "ports" {
			if p.GetValueKind() != "string" {
				return "", nil, fmt.Errorf("discovery target and ports must be strings")
			}
			if p.GetKey() == "target-host" {
				target = p.GetValue()
			} else {
				ports = p.GetValue()
			}
		}
	}
	host, err := normalizeLegionDiscoveryHost(target)
	if err != nil {
		return "", nil, err
	}
	parsed, err := parseLegionDiscoveryPorts(ports)
	return host, parsed, err
}

// Labels are user-bound release inputs, never a model-selected expansion of scope.
func legionForgeDiscoveryLabels(release *aiv1.ContextForgeRelease) (map[string]struct{}, error) {
	labels := make(map[string]struct{})
	target := ""
	for _, p := range release.GetParameters() {
		if p.GetKey() == "target-host" {
			target = p.GetValue()
		}
		if p.GetKey() != "labels" {
			continue
		}
		if p.GetValueKind() != "string" {
			return nil, fmt.Errorf("discovery labels must be a string")
		}
		parts := strings.Split(p.GetValue(), ",")
		if len(parts) > 16 {
			return nil, fmt.Errorf("discovery permits at most 16 labels")
		}
		for _, raw := range parts {
			label := strings.ToLower(strings.TrimSpace(raw))
			if !legionDiscoveryLabel(label) {
				return nil, fmt.Errorf("discovery labels must be single DNS labels")
			}
			if _, exists := labels[label]; exists {
				return nil, fmt.Errorf("discovery labels must be unique")
			}
			labels[label] = struct{}{}
		}
	}
	if len(labels) > 0 {
		host, err := normalizeLegionDiscoveryHost(target)
		if err != nil {
			return nil, err
		}
		if net.ParseIP(host) != nil {
			return nil, fmt.Errorf("discovery labels require a DNS hostname target")
		}
		for label := range labels {
			if len(label)+1+len(host) > 253 {
				return nil, fmt.Errorf("discovery label and target exceed DNS hostname length")
			}
		}
	}
	return labels, nil
}

func normalizeLegionDiscoveryHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if host == "" || len(host) > 253 || blockedLegionForgeHostname(host) {
		return "", fmt.Errorf("discovery target host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if blockedLegionForgeIP(ip) {
			return "", fmt.Errorf("discovery target address is not allowed")
		}
		return ip.String(), nil
	}
	for _, label := range strings.Split(host, ".") {
		if !legionDiscoveryLabel(label) {
			return "", fmt.Errorf("discovery requires a hostname or IP literal")
		}
	}
	return host, nil
}

func legionDiscoveryLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func parseLegionDiscoveryPorts(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || len(parts) > 32 {
		return nil, fmt.Errorf("discovery permits at most 32 ports")
	}
	ports := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		p, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || p < 1 || p > 65535 || seen[p] {
			return nil, fmt.Errorf("invalid or duplicate discovery port")
		}
		seen[p] = true
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports, nil
}

func (r *legionForgeDiscoveryRuntime) consume(n int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n < 1 || n > 256 || n > r.remaining {
		return fmt.Errorf("discovery observation budget exhausted")
	}
	r.remaining -= n
	return nil
}

func (r *legionForgeDiscoveryRuntime) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if err := r.consume(1); err != nil {
		return nil, err
	}
	literal := net.ParseIP(host)
	addresses := []net.IP{literal}
	if literal == nil {
		var err error
		addresses, err = r.lookup(ctx, host)
		if err != nil {
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
				return nil, nil
			}
			return nil, err
		}
	}
	if len(addresses) == 0 || len(addresses) > 8 {
		return nil, fmt.Errorf("discovery DNS requires 1 to 8 addresses")
	}
	result := []net.IP{}
	seen := map[string]bool{}
	for _, ip := range addresses {
		ip = normalizedLegionForgeIP(ip)
		if ip == nil || blockedLegionForgeIP(ip) || literal == nil && ip.IsPrivate() {
			return nil, fmt.Errorf("discovery DNS address is not allowed")
		}
		if !seen[ip.String()] {
			seen[ip.String()] = true
			result = append(result, ip)
		}
	}
	return result, nil
}

func newLegionForgeDiscoveryRuntime(ctx context.Context, target string, ports []int, lookup legionForgeLookupIPFunc) (*legionForgeDiscoveryRuntime, error) {
	host, err := normalizeLegionDiscoveryHost(target)
	if err != nil {
		return nil, err
	}
	if lookup == nil || len(ports) == 0 || len(ports) > 32 {
		return nil, fmt.Errorf("invalid discovery runtime scope")
	}
	for i, p := range ports {
		if p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid discovery port")
		}
		for _, prior := range ports[:i] {
			if p == prior {
				return nil, fmt.Errorf("duplicate discovery port")
			}
		}
	}
	r := &legionForgeDiscoveryRuntime{ctx: ctx, target: host, ports: append([]int(nil), ports...), lookup: lookup, dial: (&net.Dialer{}).DialContext, remaining: legionDiscoveryBudget}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r.addresses, err = r.resolve(bounded, host)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *legionForgeDiscoveryRuntime) record(kind string, result map[string]any) {
	raw, _ := json.Marshal(result)
	digest := sha256.Sum256(raw)
	operations := []string{kind}
	if kind == "dns_lookup" {
		host, _ := result["host"].(string)
		if strings.HasSuffix(host, "."+r.target) {
			label := strings.TrimSuffix(host, "."+r.target)
			if _, allowed := r.allowedLabels[label]; allowed && legionDiscoveryLabel(label) {
				operations = append(operations, "label:"+label)
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evidence = append(r.evidence, aiApplicationMaterialReference{Kind: kind, InputKey: "target-host", ResourceID: fmt.Sprintf("discovery_%x", digest[:16]), SHA256: fmt.Sprintf("%x", digest), Operations: operations})
}

func (r *legionForgeDiscoveryRuntime) applicationHTTPMaterialReferences() []aiApplicationMaterialReference {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]aiApplicationMaterialReference(nil), r.evidence...)
}

func (r *legionForgeDiscoveryRuntime) execute(ctx context.Context, name string, params map[string]any) (map[string]any, error) {
	r.mu.Lock()
	if r.calls >= 48 {
		r.mu.Unlock()
		return nil, fmt.Errorf("discovery call budget exceeded")
	}
	r.calls++
	r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	if err := bounded.Err(); err != nil {
		return nil, err
	}
	// Host and port arguments are never accepted, even if a model invents them.
	for key := range params {
		// ToolCaller injects runtime_id and Tool.InvokeWithParams restores it
		// after schema validation. It is bookkeeping, never a target input.
		if key == "runtime_id" {
			continue
		}
		if key != "label" || name != "dns_lookup" {
			return nil, fmt.Errorf("discovery argument %q cannot alter the authorized scope", key)
		}
	}
	result := map[string]any{"target": r.target, "observed_at": time.Now().UTC().Format(time.RFC3339Nano)}
	switch name {
	case "dns_lookup":
		host := r.target
		addresses := r.addresses
		label := strings.ToLower(strings.TrimSpace(utils.InterfaceToString(params["label"])))
		if label != "" {
			if _, allowed := r.allowedLabels[label]; !allowed {
				return nil, fmt.Errorf("DNS label is outside the user-authorized labels")
			}
			if net.ParseIP(r.target) != nil || !legionDiscoveryLabel(label) {
				return nil, fmt.Errorf("DNS label must be one bounded subdomain label")
			}
			host = label + "." + r.target
			if len(host) > 253 || blockedLegionForgeHostname(host) {
				return nil, fmt.Errorf("DNS label target is not allowed")
			}
			var err error
			addresses, err = r.resolve(bounded, host)
			if err != nil {
				return nil, err
			}
		} else if err := r.consume(1); err != nil {
			return nil, err
		}
		ips := []string{}
		for _, ip := range addresses {
			ips = append(ips, ip.String())
		}
		result["host"] = host
		result["addresses"] = ips
		result["source"] = "pinned_dns_resolution"
		if len(addresses) == 0 {
			result["status"] = "not_found"
		} else {
			result["status"] = "resolved"
		}
	case "tcp_connect_scan":
		cost := len(r.addresses) * len(r.ports)
		if cost == 0 {
			cost = 1
			result["status"] = "no_resolved_addresses"
		}
		if err := r.consume(cost); err != nil {
			return nil, err
		}
		observations := []map[string]any{}
		for _, ip := range r.addresses {
			for _, port := range r.ports {
				if err := bounded.Err(); err != nil {
					return nil, err
				}
				attempt, done := context.WithTimeout(bounded, time.Second)
				conn, err := r.dial(attempt, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
				done()
				item := map[string]any{"address": ip.String(), "port": port, "connected": err == nil}
				if conn != nil {
					conn.Close()
				}
				if err != nil {
					item["error"] = err.Error()
				}
				observations = append(observations, item)
			}
		}
		result["observations"] = observations
		result["source"] = "tcp_connect_without_payload"
	default:
		return nil, fmt.Errorf("unsupported discovery operation")
	}
	if err := bounded.Err(); err != nil {
		return nil, err
	}
	r.record(name, result)
	return result, nil
}

func legionForgeDiscoveryOptions(ctx context.Context, release *aiv1.ContextForgeRelease) ([]aicommon.ConfigOption, legionForgeMaterialRuntime, error) {
	host, ports, err := legionForgeDiscoveryParameters(release)
	if err != nil {
		return nil, nil, err
	}
	runtime, err := newLegionForgeDiscoveryRuntime(ctx, host, ports, defaultLegionForgeLookupIP)
	if err != nil {
		return nil, nil, err
	}
	runtime.allowedLabels, err = legionForgeDiscoveryLabels(release)
	if err != nil {
		return nil, nil, err
	}
	tools := []*aitool.Tool{}
	for _, name := range legionForgeDiscoveryTools {
		name := name
		opts := []aitool.ToolOption{aitool.WithDescription("Observe only the fixed authorized target. DNS accepts one optional subdomain label; TCP connects only to the fixed ports without payloads or login."), aitool.WithNoRuntimeCallback(func(callCtx context.Context, params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return runtime.execute(callCtx, name, map[string]any(params))
		})}
		if name == "dns_lookup" {
			opts = append(opts, aitool.WithStringParam("label"))
		}
		tool, err := aitool.New(name, opts...)
		if err != nil {
			return nil, nil, err
		}
		tools = append(tools, tool)
	}
	return restrictedLegionForgeToolOptions(tools), runtime, nil
}
