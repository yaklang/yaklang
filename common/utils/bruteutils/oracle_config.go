package bruteutils

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/oracleprobe"
)

// OracleConfig configures password login without importing a database driver.
// Services defaults to the built-in candidates. SID selects SID instead of
// SERVICE_NAME. Nil SysDBA automatically uses SYSDBA only for user SYS.
// TLS nil uses TCP; a non-nil config requires TCPS and never falls back to TCP.
// Encryption is accepted (default), rejected, requested or required.
// Timeout covers ALL services and retries, defaults to 10s and is capped at 20s.
type OracleConfig struct {
	Services   []string
	SID        bool
	SysDBA     *bool
	TLS        *tls.Config
	Encryption string
	Timeout    time.Duration
}

func (o *OracleConfig) clone() *OracleConfig {
	if o == nil {
		return nil
	}
	c := *o
	c.Services = append([]string(nil), o.Services...)
	if o.SysDBA != nil {
		v := *o.SysDBA
		c.SysDBA = &v
	}
	if o.TLS != nil {
		c.TLS = o.TLS.Clone()
		if o.TLS.RootCAs != nil {
			c.TLS.RootCAs = o.TLS.RootCAs.Clone()
		}
	}
	return &c
}

func (o *OracleConfig) normalize() ([]string, oracleprobe.EncryptionPolicy, time.Duration, error) {
	services := oracleServiceNames
	policy, timeout := oracleprobe.EncryptionAccepted, defaultTimeout
	if o == nil {
		return append([]string(nil), services...), policy, timeout, nil
	}
	switch strings.ToLower(o.Encryption) {
	case "", "accepted":
	case "rejected":
		policy = oracleprobe.EncryptionRejected
	case "requested":
		policy = oracleprobe.EncryptionRequested
	case "required":
		policy = oracleprobe.EncryptionRequired
	default:
		return nil, policy, timeout, fmt.Errorf("oracle: invalid encryption policy")
	}
	if len(o.Services) > 0 {
		services = o.Services
	}
	if len(services) > 32 {
		return nil, policy, timeout, fmt.Errorf("oracle: at most 32 service candidates are allowed")
	}
	if o.SID && len(o.Services) == 0 {
		return nil, policy, timeout, fmt.Errorf("oracle: SID requires an explicit name")
	}
	if o.Timeout < 0 {
		return nil, policy, timeout, fmt.Errorf("oracle: timeout must be positive")
	}
	if o.Timeout > 0 {
		timeout = min(o.Timeout, oracleprobe.MaxTimeout)
	}
	result := make([]string, 0, len(services))
	seen := make(map[string]bool)
	for _, s := range services {
		if s == "" || len(s) > 1024 || strings.ContainsAny(s, "()\x00\r\n") {
			return nil, policy, timeout, fmt.Errorf("oracle: invalid service name")
		}
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}
	return result, policy, timeout, nil
}

// Validate checks configuration before a task opens any connections.
func (o *OracleConfig) Validate() error { _, _, _, err := o.normalize(); return err }

// WithOracleConfig configures the built-in Oracle handler for this BruteUtil.
func WithOracleConfig(config *OracleConfig) OptionsAction {
	snapshot := config.clone()
	return func(b *BruteUtil) { b.oracleConfig = snapshot.clone() }
}

type oracleCacheKey struct{}
type oracleServiceKey struct {
	target, service string
	sid             bool
}
type oracleServiceObservation struct {
	unknown bool
	expires time.Time
}

const oracleCacheLimit = 4096
const oracleCacheTTL = 30 * time.Second

// The cache belongs to one scan context, stores no credentials, and only omits
// services explicitly rejected as unknown. Known services never hide others.
type oracleServiceCache struct {
	mu      sync.Mutex
	entries map[oracleServiceKey]oracleServiceObservation
}

func withOracleServiceCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, oracleCacheKey{}, &oracleServiceCache{entries: make(map[oracleServiceKey]oracleServiceObservation)})
}
func (c *oracleServiceCache) observation(k oracleServiceKey) (oracleServiceObservation, bool) {
	if c == nil {
		return oracleServiceObservation{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[k]
	if ok && !time.Now().Before(v.expires) {
		delete(c.entries, k)
		return oracleServiceObservation{}, false
	}
	return v, ok
}
func (c *oracleServiceCache) remember(k oracleServiceKey, unknown bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[k]; !ok && len(c.entries) >= oracleCacheLimit {
		for old := range c.entries {
			delete(c.entries, old)
			break
		}
	}
	c.entries[k] = oracleServiceObservation{unknown: unknown, expires: time.Now().Add(oracleCacheTTL)}
}
