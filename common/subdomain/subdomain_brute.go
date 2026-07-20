package subdomain

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// aRecordQuerier is the minimal surface of the scanner used by brute force
// and wildcard detection to issue A-record queries. It is an interface so the
// wildcard algorithm can be unit-tested with a fake resolver instead of real
// DNS (which is non-deterministic, network-bound, and unreliable in CI).
type aRecordQuerier interface {
	QueryA(ctx context.Context, domain string) (ip string, server string, _ error)
}

// WildcardMode classifies the outcome of wildcard DNS detection.
type WildcardMode int

const (
	// WildcardNone: random probes do not resolve (or only some resolve).
	// The target is treated as non-wildcard; brute force proceeds normally.
	WildcardNone WildcardMode = iota
	// WildcardSingleIP: all probes resolve and every probe returns the *same*
	// IP. This is the classic operator/self-configured wildcard (泛解析),
	// pointing all non-existent labels at a single landing IP. Brute force
	// proceeds with that IP blacklisted (unless WildCardToStop is set).
	WildcardSingleIP
	// WildcardHijacked: all probes resolve but return *multiple distinct* IPs.
	// This indicates DNS has been taken over / hijacked (e.g. a local TUN mode
	// intercepting DNS). Brute force is meaningless: dictionary results would
	// all be forged and none would be filtered by a fixed blacklist. The
	// scan is aborted with a clear reason.
	WildcardHijacked
)

func (s *SubdomainScanner) brute(
	ctx context.Context, domain string,
	useSubDictionary bool,
	callback func(domain string, ip string, fromServer string),
	blacklistIP []string,
) {
	if ctx.Err() != nil {
		return
	}
	var handler func(ctx context.Context) chan string
	if !useSubDictionary {
		s.logger.Infof("use main dictionary for %s", domain)
		handler = func(c context.Context) chan string {
			return generateDictionary(c, s.config.MainDictionary)
		}
	} else {
		s.logger.Infof("use sub dictionary for %s", domain)
		handler = func(c context.Context) chan string {
			return generateDictionary(c, s.config.SubDictionary)
		}
	}

	querier := s.aRecordQuerier()
	if querier == nil {
		querier = s
	}

	wg := sync.WaitGroup{}
	for line := range handler(ctx) {

		line := line
		err := s.dnsQuerierSwg.AddWithContext(ctx)
		if err != nil {
			return
		}
		wg.Add(1)
		go func(raw string) {
			defer s.dnsQuerierSwg.Done()
			defer wg.Done()

			target := fmt.Sprintf("%s.%s", raw, domain)
			target = formatDomain(target)
			s.logger.Debugf("start to query %s", target)
			ip, server, err := querier.QueryA(ctx, target)
			if err != nil {
				s.logger.Debugf("query [%s] A failed: %s", target, err)
				return
			}

			// 设置黑名单，过滤统配情况
			for _, bip := range blacklistIP {
				if bip == ip {
					s.logger.Debugf("maybe [%s] - [%s] from %s is detected by wildcard checking", target, ip, server)
					return
				}
			}
			callback(target, ip, server)
		}(line)
	}

	wg.Wait()
}

func (s *SubdomainScanner) Brute(ctx context.Context, target string) {
	s.BruteWithSubDictionarySelection(ctx, target, false, 0)
}

func (s *SubdomainScanner) BruteWithSubDictionarySelection(ctx context.Context, target string, useSubDictionary bool, depth int) {
	if s.config.MaxDepth > 0 && s.config.MaxDepth <= depth {
		s.logger.Debugf("the domain %s is skipped for depth: %v", target, depth)
		return
	}

	target = formatDomain(target)

	if ctx.Err() != nil {
		return
	}

	s.logger.Infof("start to checking wildcard for %s", target)
	mode, tested, blacklistIP := s.isWildCard(ctx, target)
	switch mode {
	case WildcardHijacked:
		// DNS 疑似被劫持/接管（例如本地开启了 TUN 模式劫持 DNS）：
		// 所有不存在的随机子域名都解析成功，但返回的 IP 各不相同，
		// 说明真实字典的爆破结果也会被伪造且无法通过固定黑名单过滤，
		// 继续爆破没有意义。立即中止并通知调用方。
		reason := fmt.Sprintf(
			"subdomain brute aborted for %s: all %d random probes resolved but returned %d distinct IPs (e.g. %s) — DNS appears to be hijacked/taken over (e.g. local TUN mode). Brute force is meaningless here.",
			target, len(tested), len(blacklistIP), strings.Join(tested, ", "),
		)
		s.logger.Errorf(reason)
		s.onScanAborted(reason)
		return
	case WildcardSingleIP:
		s.logger.Infof("maybe %s has dns wildcard resolving setting, tested: [%s]", target, strings.Join(tested, "|"))
		s.logger.Infof("we detected wildcard ip blacklist [%s]", strings.Join(blacklistIP, " | "))
		if s.config.WildCardToStop {
			return
		}
	case WildcardNone:
		// 无泛解析，正常爆破
	}

	if ctx.Err() != nil {
		return
	}

	s.logger.Infof("start to brute subdomain for %s", target)

	wg := sync.WaitGroup{}
	s.brute(ctx, target, useSubDictionary,

		// 设置回调
		func(domain string, ip string, fromServer string) {
			s.onResult(&SubdomainResult{
				FromDNSServer: fromServer,
				FromModeRaw:   BRUTE,
				FromTarget:    target,
				IP:            ip,
				Domain:        domain,
			})

			// 如果不允许递归则，退出回调
			if !s.config.AllowToRecursive {
				return
			}

			// 如果允许递归，则继续调用该函数进行递归
			nxtDepth := depth + 1
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.BruteWithSubDictionarySelection(ctx, domain, true, nxtDepth)
			}()
		},

		// 这里是 IP 黑名单
		blacklistIP,
	)

	wg.Wait()
}

// aRecordQuerier returns the override querier if set, otherwise nil.
// nil callers fall back to the scanner itself (which implements QueryA).
func (s *SubdomainScanner) aRecordQuerier() aRecordQuerier {
	return s.querierOverride
}

// isWildCard probes a target with several random, guaranteed-not-to-exist
// subdomains and classifies the wildcard behavior.
//
// Decision tree (N = probe count, resolved = how many probes returned an A
// record, distinctIPs = number of distinct IPs among those):
//
//	resolved == 0              -> WildcardNone (no wildcard)
//	0 < resolved < N           -> WildcardNone (partial hit, avoid false positive)
//	resolved == N, distinct==1 -> WildcardSingleIP (classic 泛解析)
//	resolved == N, distinct>1  -> WildcardHijacked (DNS taken over / hijacked)
//
// For WildcardSingleIP the blacklist is [the single IP]; for WildcardHijacked
// the blacklist is the set of distinct IPs (returned for diagnostics only —
// the caller aborts and does not use it as a filter); for WildcardNone it is
// empty.
func (s *SubdomainScanner) isWildCard(ctx context.Context, target string) (mode WildcardMode, tested []string, blacklist []string) {
	n := s.config.WildCardProbeCount
	if n <= 0 {
		n = 10
	}

	querier := s.aRecordQuerier()
	if querier == nil {
		querier = s
	}

	var (
		mu         sync.Mutex
		ipSet      = map[string]struct{}{}
		testedOnce []string
	)

	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		payload := fmt.Sprintf("%s.%s", utils.RandStringBytes(16), target)

		err := s.dnsQuerierSwg.AddWithContext(ctx)
		if err != nil {
			return WildcardNone, nil, nil
		}

		wg.Add(1)
		go func() {
			defer s.dnsQuerierSwg.Done()
			defer wg.Done()

			ip, _, err := querier.QueryA(ctx, payload)
			if err != nil {
				s.logger.Debugf("wildcard detected: checking %s failed: %s", payload, err)
				return
			}

			mu.Lock()
			testedOnce = append(testedOnce, payload)
			ipSet[ip] = struct{}{}
			mu.Unlock()
		}()
	}

	wg.Wait()

	if len(testedOnce) == 0 {
		return WildcardNone, nil, nil
	}

	for ip := range ipSet {
		blacklist = append(blacklist, ip)
	}

	// 全部命中 -> 泛解析或劫持
	if len(testedOnce) == n {
		if len(ipSet) == 1 {
			// 经典泛解析：所有不存在子域名都指向同一个 IP。
			// 但需进一步排除“单 IP sinkhole 劫持”（如本地 TUN 模式
			// 把所有 DNS 都劫持到同一个伪 IP）：抽样几个真实子域名前缀，
			// 若也都解析到同一个 wildcard IP，则升级为 WildcardHijacked。
			var wildcardIP string
			for ip := range ipSet {
				wildcardIP = ip
			}
			if s.config.WildCardSinkholeVerify && s.isSinkholeHijack(ctx, querier, target, wildcardIP) {
				return WildcardHijacked, testedOnce, blacklist
			}
			return WildcardSingleIP, testedOnce, blacklist
		}
		// 所有不存在子域名都解析成功但 IP 各不相同 -> DNS 被接管/劫持
		return WildcardHijacked, testedOnce, blacklist
	}

	// 部分命中：可能是偶发，避免误报，视为无泛解析
	return WildcardNone, testedOnce, []string{}
}

// sinkholeSampleLabels 是用于 sinkhole 验证的“真实子域名前缀”抽样。
// 这些是公共互联网上极常见的标签，合法域名往往有其中至少一个解析到
// 与泛解析落地 IP 不同的地址；若全部都解析到同一个 wildcard IP，
// 则强烈提示 DNS 被劫持到单一 sinkhole。
var sinkholeSampleLabels = []string{"www", "mail", "ftp", "ns1"}

// isSinkholeHijack 在经典泛解析（单 IP）判定后做抽样验证：
// 取几个真实子域名前缀并发查询 A 记录，若全部解析到同一个 wildcardIP，
// 则认为是单 IP sinkhole 劫持，返回 true（调用方据此升级为 WildcardHijacked）。
// 只要有一个抽样词解析失败或解析到不同 IP，就认为是真实泛解析，返回 false。
func (s *SubdomainScanner) isSinkholeHijack(ctx context.Context, querier aRecordQuerier, target, wildcardIP string) bool {
	var (
		mu      sync.Mutex
		allSame = true
	)
	wg := sync.WaitGroup{}
	for _, label := range sinkholeSampleLabels {
		payload := fmt.Sprintf("%s.%s", label, target)

		if err := s.dnsQuerierSwg.AddWithContext(ctx); err != nil {
			// 受限退出时无法判定，保守视为非 sinkhole（不误报）。
			return false
		}

		wg.Add(1)
		go func(p string) {
			defer s.dnsQuerierSwg.Done()
			defer wg.Done()

			ip, _, err := querier.QueryA(ctx, p)
			if err != nil {
				// 真实子域名解析失败（NXDOMAIN）-> 不是 sinkhole 劫持。
				mu.Lock()
				allSame = false
				mu.Unlock()
				return
			}
			if ip != wildcardIP {
				// 解析到不同 IP -> 是真实泛解析（有真实记录）。
				mu.Lock()
				allSame = false
				mu.Unlock()
			}
		}(payload)
	}
	wg.Wait()
	return allSame
}

// UseQuerier installs an override A-record querier used by brute force and
// wildcard detection. It is primarily intended for tests; production code
// should leave it unset so the scanner queries real DNS via QueryA.
func (s *SubdomainScanner) UseQuerier(q aRecordQuerier) {
	s.querierOverride = q
}

// querierOverride holds an optional A-record querier override (see UseQuerier).
var _ aRecordQuerier = (*SubdomainScanner)(nil)
