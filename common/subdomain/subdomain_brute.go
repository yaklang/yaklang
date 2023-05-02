package subdomain

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"github.com/yaklang/yaklang/common/utils"
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
			ip, server, err := s.QueryA(ctx, target)
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

	var (
		ok                  bool
		tested, blacklistIP []string
	)

	if ctx.Err() != nil {
		return
	}

	s.logger.Infof("start to checking wildcard for %s", target)
	if ok, tested, blacklistIP = s.isWildCard(ctx, target); ok {
		s.logger.Infof("maybe %s has dns wildcard resolving setting, tested: [%s]", target, strings.Join(tested, "|"))
		s.logger.Infof("we detected blacklist ip [%s]", strings.Join(blacklistIP, " | "))

		if s.config.WildCardToStop {
			return
		}
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

func (s *SubdomainScanner) isWildCard(ctx context.Context, target string) (ok bool, tested []string, blacklist []string) {
	result := sync.Map{}

	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		payload := fmt.Sprintf("%s.%s", utils.RandStringBytes(16), target)

		err := s.dnsQuerierSwg.AddWithContext(ctx)
		if err != nil {
			return false, nil, nil
		}

		wg.Add(1)
		go func() {
			defer s.dnsQuerierSwg.Done()
			defer wg.Done()

			ip, _, err := s.QueryA(ctx, payload)
			if err != nil {
				s.logger.Debugf("wildcard detected: checking %s failed: %s", payload, err)
				return
			}
			tested = append(tested, payload)

			result.Store(ip, 0)
		}()
	}

	wg.Wait()

	result.Range(func(key, value interface{}) bool {
		blacklist = append(blacklist, key.(string))
		return true
	})

	if len(tested) >= 5 {
		return true, tested, blacklist
	}
	return false, []string{}, []string{}
}
