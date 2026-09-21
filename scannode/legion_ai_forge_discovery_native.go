package scannode

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/subdomain"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// The native scanner never owns an outbound DNS socket here. Every A query
// flows through the same reviewed resolver/address policy as the v1 tools.
type legionForgeNativeQuerier struct {
	runtime      *legionForgeDiscoveryRuntime
	mu           sync.Mutex
	controls     int
	seen         map[string]bool
	observations []map[string]any
	failure      error
}

func (q *legionForgeNativeQuerier) QueryA(ctx context.Context, host string) (string, string, error) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if !strings.HasSuffix(host, "."+q.runtime.target) {
		return "", "", fmt.Errorf("native DNS escaped target")
	}
	label := strings.TrimSuffix(host, "."+q.runtime.target)
	_, authorized := q.runtime.allowedLabels[label]
	q.mu.Lock()
	if !legionDiscoveryLabel(label) || q.seen[label] || (!authorized && (len(label) != 16 || q.controls >= 2)) {
		err := fmt.Errorf("native DNS escaped dictionary or wildcard budget")
		q.failure = err
		q.mu.Unlock()
		return "", "", err
	}
	q.seen[label] = true
	if !authorized {
		q.controls++
	}
	q.mu.Unlock()
	addresses, err := q.runtime.resolve(ctx, host)
	observation := map[string]any{"target": q.runtime.target, "host": host, "source": "native_subdomain_brute", "observed_at": time.Now().UTC().Format(time.RFC3339Nano)}
	ips := []string{}
	for _, ip := range addresses {
		if ip.To4() != nil {
			ips = append(ips, ip.String())
		}
	}
	observation["addresses"] = ips
	if err != nil {
		observation["status"] = "error"
		observation["error"] = err.Error()
	} else if len(ips) == 0 {
		observation["status"] = "not_found"
	} else {
		observation["status"] = "resolved"
	}
	q.mu.Lock()
	if err != nil && q.failure == nil {
		q.failure = err
	}
	q.observations = append(q.observations, observation)
	q.mu.Unlock()
	if authorized && err == nil {
		q.runtime.record("dns_lookup", observation)
	}
	if err != nil {
		return "", "", err
	}
	if len(ips) == 0 {
		return "", "", &net.DNSError{Name: host, Err: "no A record", IsNotFound: true}
	}
	return ips[0], "managed-resolver", nil
}

func (r *legionForgeDiscoveryRuntime) enumerateNative(ctx context.Context) (map[string]any, error) {
	labels := make([]string, 0, len(r.allowedLabels))
	for label := range r.allowedLabels {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	if len(labels) == 0 || len(labels) > 16 || len(r.target) > 236 || net.ParseIP(r.target) != nil {
		return nil, fmt.Errorf("invalid native enumeration scope")
	}
	cfg := subdomain.NewSubdomainScannerConfig(subdomain.WithModes(subdomain.BRUTE))
	cfg.AllowToRecursive = false
	cfg.MaxDepth = 1
	cfg.WorkerCount = 1
	cfg.ParallelismTasksCount = 1
	cfg.MainDictionary = []byte(strings.Join(labels, "\n"))
	cfg.SubDictionary = nil
	cfg.DNSServers = nil // The mandatory bound resolver below is the only DNS path.
	cfg.WildCardProbeCount = 2
	cfg.WildCardToStop = true
	cfg.WildCardSinkholeVerify = false
	cfg.TimeoutForEachTarget = 15 * time.Second
	cfg.TimeoutForEachQuery = time.Second
	scanner, err := subdomain.NewSubdomainScanner(cfg, r.target)
	if err != nil {
		return nil, err
	}
	query := &legionForgeNativeQuerier{runtime: r, seen: map[string]bool{}}
	scanner.SetARecordQuerier(query)
	matches := []map[string]any{}
	var mu sync.Mutex
	aborted := ""
	scanner.OnResult(func(result *subdomain.SubdomainResult) {
		mu.Lock()
		defer mu.Unlock()
		matches = append(matches, map[string]any{"host": result.Domain, "address": result.IP})
	})
	scanner.OnScanAborted(func(reason string) { mu.Lock(); defer mu.Unlock(); aborted = reason })
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := scanner.RunWithContext(runCtx); err != nil {
		return nil, err
	}
	if err := runCtx.Err(); err != nil {
		return nil, err
	}
	query.mu.Lock()
	defer query.mu.Unlock()
	if query.failure != nil {
		return nil, fmt.Errorf("native DNS observation failed: %w", query.failure)
	}
	if aborted != "" {
		return nil, fmt.Errorf("native subdomain enumeration aborted: %s", aborted)
	}
	for _, label := range labels {
		if !query.seen[label] {
			return nil, fmt.Errorf("native subdomain enumeration stopped before dictionary completion (wildcard or cancellation)")
		}
	}
	sort.Slice(query.observations, func(i, j int) bool {
		return query.observations[i]["host"].(string) < query.observations[j]["host"].(string)
	})
	sort.Slice(matches, func(i, j int) bool { return matches[i]["host"].(string) < matches[j]["host"].(string) })
	return map[string]any{"target": r.target, "source": "native_subdomain_brute", "labels": labels, "recursion": false, "wildcard_probe_count": query.controls, "observations": query.observations, "matches": matches, "complete_dictionary": true, "exhaustive": false}, nil
}
