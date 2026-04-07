package loop_infosec_recon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

const poolFileName = "recon_api_pool.json"
const poolFormatVersion = 1

// APIPoolEntry is one deduplicated API candidate in the shared pool.
type APIPoolEntry struct {
	NormalizedURL string  `json:"normalized_url"`
	Method        string  `json:"method,omitempty"`
	Source        string  `json:"source"`
	Confidence    float64 `json:"confidence,omitempty"`
	Evidence      string  `json:"evidence,omitempty"`
	Verified      bool    `json:"verified"`
	StatusCode    int     `json:"status_code,omitempty"`
	ProbeError    string  `json:"probe_error,omitempty"`
}

// APIPool is persisted under the task work directory.
type APIPool struct {
	Version int            `json:"version"`
	SeedURL string         `json:"seed_url,omitempty"`
	Entries []APIPoolEntry `json:"entries"`
}

// entryKey returns a dedupe key (method upper + URL).
func entryKey(method, normalizedURL string) string {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		m = "GET"
	}
	return m + "\x00" + normalizedURL
}

// NormalizeURL trims space, removes fragment, lowercases host; returns absolute URL when possible.
func NormalizeURL(raw, baseSeed string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", utils.Error("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		if baseSeed == "" {
			return "", utils.Errorf("relative url %q needs seed URL", raw)
		}
		bu, err := url.Parse(baseSeed)
		if err != nil {
			return "", err
		}
		u = bu.ResolveReference(u)
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// LoadAPIPool reads the pool from workDir.
func LoadAPIPool(workDir string) (*APIPool, error) {
	path := filepath.Join(workDir, poolFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &APIPool{Version: poolFormatVersion, Entries: []APIPoolEntry{}}, nil
		}
		return nil, err
	}
	var p APIPool
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if p.Version == 0 {
		p.Version = poolFormatVersion
	}
	if p.Entries == nil {
		p.Entries = []APIPoolEntry{}
	}
	return &p, nil
}

// SaveAPIPool writes the pool to workDir.
func SaveAPIPool(workDir string, p *APIPool) error {
	if p == nil {
		return utils.Error("nil pool")
	}
	p.Version = poolFormatVersion
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(workDir, poolFileName)
	return os.WriteFile(path, data, 0644)
}

// ParseScopeHostSet parses comma-separated hostnames (lowercase) for authorized-scope checks.
func ParseScopeHostSet(scopeCSV string) map[string]bool {
	m := make(map[string]bool)
	for _, p := range strings.Split(scopeCSV, ",") {
		h := strings.ToLower(strings.TrimSpace(p))
		if h != "" {
			m[h] = true
		}
	}
	return m
}

func entryHostInScope(normalizedURL string, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	u, err := url.Parse(normalizedURL)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return allowed[h]
}

// MergeFindings merges new rows into an in-memory pool (dedupe by method+normalized URL).
// If scopeHostsCSV is non-empty, entries whose host is not in the set are skipped.
func MergeFindings(p *APIPool, seedURL string, findings []struct {
	URL        string
	Method     string
	Source     string
	Evidence   string
	Confidence float64
}, scopeHostsCSV ...string) (added int, errs []string) {
	if p == nil {
		return 0, []string{"nil pool"}
	}
	var allowed map[string]bool
	if len(scopeHostsCSV) > 0 && strings.TrimSpace(scopeHostsCSV[0]) != "" {
		allowed = ParseScopeHostSet(scopeHostsCSV[0])
	}
	seen := make(map[string]struct{}, len(p.Entries))
	for _, e := range p.Entries {
		seen[entryKey(e.Method, e.NormalizedURL)] = struct{}{}
	}
	for _, f := range findings {
		norm, err := NormalizeURL(f.URL, seedURL)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if !entryHostInScope(norm, allowed) {
			continue
		}
		m := strings.TrimSpace(f.Method)
		if m == "" {
			m = "GET"
		}
		k := entryKey(m, norm)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		p.Entries = append(p.Entries, APIPoolEntry{
			NormalizedURL: norm,
			Method:        strings.ToUpper(m),
			Source:        f.Source,
			Evidence:      f.Evidence,
			Confidence:    f.Confidence,
		})
		added++
	}
	return added, errs
}

// PoolStats returns counts for reactive UI.
func PoolStats(p *APIPool) (total, verified, unverified int, bySource map[string]int) {
	bySource = make(map[string]int)
	if p == nil {
		return 0, 0, 0, bySource
	}
	for _, e := range p.Entries {
		total++
		if e.Verified {
			verified++
		} else {
			unverified++
		}
		bySource[e.Source]++
	}
	return total, verified, unverified, bySource
}

// ProbePoolHTTP issues HEAD or GET for unverified entries (limited).
// If allowedHosts is non-empty, only entries whose URL host is in the set are probed.
func ProbePoolHTTP(p *APIPool, limit, concurrency int, useHead bool, timeout time.Duration, allowedHosts map[string]bool) int {
	if p == nil || limit <= 0 {
		return 0
	}
	client := &http.Client{Timeout: timeout}
	var targets []*APIPoolEntry
	for i := range p.Entries {
		if p.Entries[i].Verified {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(p.Entries[i].NormalizedURL), "http") {
			continue
		}
		if !entryHostInScope(p.Entries[i].NormalizedURL, allowedHosts) {
			continue
		}
		targets = append(targets, &p.Entries[i])
		if len(targets) >= limit {
			break
		}
	}
	if len(targets) == 0 {
		return 0
	}
	if concurrency < 1 {
		concurrency = 4
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, e := range targets {
		e := e
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			req, err := http.NewRequest(http.MethodGet, e.NormalizedURL, nil)
			if useHead {
				req, err = http.NewRequest(http.MethodHead, e.NormalizedURL, nil)
			}
			if err != nil {
				e.ProbeError = err.Error()
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				e.ProbeError = err.Error()
				return
			}
			_ = resp.Body.Close()
			e.StatusCode = resp.StatusCode
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
				e.Verified = true
				e.ProbeError = ""
			} else {
				e.Verified = false
				if resp.Status != "" {
					e.ProbeError = resp.Status
				} else {
					e.ProbeError = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}
		}()
	}
	wg.Wait()
	return len(targets)
}

// ExtractFromJSReport parses js_static_extract_ai tool final_report.json for API rows.
func ExtractFromJSReport(data []byte) []struct {
	URL, Method, Evidence, Source string
	Confidence                    float64
} {
	var out []struct {
		URL, Method, Evidence, Source string
		Confidence                    float64
	}
	var top struct {
		ApisFinal     []map[string]interface{} `json:"apis_final"`
		ApisMergedMap map[string]interface{}   `json:"apis_merged_map"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return out
	}
	appendOne := func(m map[string]interface{}) {
		fu := strings.TrimSpace(utils.InterfaceToString(m["full_url"]))
		hm := strings.TrimSpace(utils.InterfaceToString(m["http_method"]))
		if fu == "" || hm == "" {
			return
		}
		ev := utils.InterfaceToString(m["evidence"])
		out = append(out, struct {
			URL, Method, Evidence, Source string
			Confidence                    float64
		}{
			URL: fu, Method: hm, Evidence: ev, Source: "js_static",
			Confidence: 0.8,
		})
	}
	for _, m := range top.ApisFinal {
		if m != nil {
			appendOne(m)
		}
	}
	for _, v := range top.ApisMergedMap {
		vm, _ := v.(map[string]interface{})
		if vm != nil {
			appendOne(vm)
		}
	}
	return out
}
