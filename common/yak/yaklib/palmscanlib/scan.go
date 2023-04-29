package palmscanlib

import (
	"context"
	"net/http"
	"yaklang/common/fp"
	"yaklang/common/fp/webfingerprint"
	//"palm/common/hybridscan"
	"yaklang/common/log"
	"yaklang/common/subdomain"
	"yaklang/common/utils"
)

var (
	ScanExports = map[string]interface{}{
		"ConvertHTTPResponseToHTTPResponseInfo": webfingerprint.ExtractHTTPResponseInfoFromHTTPResponse,
		"HTTPResponseToMatchResult":             scanHTTPResponseToMatchResult,

		// 扫描子域名
		"ScanSubDomain":                         ScanSubDomainQuick,
		"ScanSubDomainWithConfig":               scanSubDomain,
		"WithSubDomainAllowToRecursive":         subdomain.WithAllowToRecursive,
		"WithSubDomainDNSServers":               subdomain.WithDNSServers,
		"WithSubDomainMainDictionary":           subdomain.WithMainDictionary,
		"WithSubDomainMaxDepth":                 subdomain.WithMaxDepth,
		"WithSubDomainModes":                    subdomain.WithModes,
		"WithSubDomainParallelismTasksCount":    subdomain.WithParallelismTasksCount,
		"WithSubDomainSubDictionary":            subdomain.WithSubDictionary,
		"WithSubDomainTimeoutForEachHTTPSearch": subdomain.WithTimeoutForEachHTTPSearch,
		"WithSubDomainTimeoutForEachQuery":      subdomain.WithTimeoutForEachQuery,
		"WithSubDomainTimeoutForEachTarget":     subdomain.WithTimeoutForEachTarget,
		"WithSubDomainWildCardToStop":           subdomain.WithWildCardToStop,
		"WithSubDomainWorkerCount":              subdomain.WithWorkerCount,
		"GetDefaultScanSubDomainConfig":         subdomain.NewSubdomainScannerConfig,

		//"HybridScanPortWithConfig": scanPort,
		//"HybridScanPort": func(ctx context.Context, host, port string,
		//	cb func(ip, port interface{}),
		//	fpCallback func(result interface{}),
		//) error {
		//	config, err := hybridscan.NewDefaultConfig()
		//	if err != nil {
		//		return utils.Errorf("create hybridscan config failed: %s", err)
		//	}
		//
		//	return scanPort(ctx, config, host, port, cb, fpCallback)
		//},
		//"GetDefaultHybridScanConfig":                    hybridscan.NewDefaultConfig,
		//"WithHybridScanOpenPortTTLCache":                hybridscan.WithOpenPortTTLCache,
		//"WithHybridScanFingerprintMatchQueueBufferSize": hybridscan.WithFingerprintMatchQueueBufferSize,
		//"WithHybridScanDisableFingerprintMatch":         hybridscan.WithDisableFingerprintMatch,
		//"WithHybridScanFingerprintMatcherConfig":        hybridscan.WithFingerprintMatcherConfig,
		//"WithHybridScanFingerprintMatcherConfigOptions": hybridscan.WithFingerprintMatcherConfigOptions,
		//"WithHybridScanFingerprintMatchResultTTLCache":  hybridscan.WithFingerprintMatchResultTTLCache,
		//"WithHybridScanSynScanConfig":                   hybridscan.WithSynScanConfig,
	}
)

var (
	defaultHTTPResponseMatcher *webfingerprint.Matcher
)

func scanHTTPResponseToMatchResult(r *http.Response) ([]*webfingerprint.CPE, error) {
	if defaultHTTPResponseMatcher == nil {
		rules, err := fp.GetDefaultWebFingerprintRules()
		if err != nil {
			return nil, utils.Errorf("get web rules failed: %s", err)
		}
		defaultHTTPResponseMatcher, err = webfingerprint.NewWebFingerprintMatcher(rules, false, true)
		if err != nil {
			return nil, utils.Errorf("create matcher failed: %s", err)
		}
	}

	info := webfingerprint.ExtractHTTPResponseInfoFromHTTPResponseWithBodySize(r, 2048)
	return defaultHTTPResponseMatcher.Match(info)
}

//func scanPort(
//	ctx context.Context, config *hybridscan.Config,
//	host string, port string, portCallback func(interface{}, interface{}),
//	fpCallback func(interface{}),
//) error {
//	center, err := hybridscan.NewHyperScanCenter(ctx, config)
//	if err != nil {
//		return err
//	}
//
//	rid, err := uuid.NewV4()
//	if err != nil {
//		return err
//	}
//	err = center.RegisterMatcherResultHandler(rid.String(), func(matcherResult *fp.MatchResult, err error) {
//		if err != nil {
//			return
//		}
//
//		if fpCallback != nil {
//			fpCallback(matcherResult)
//		}
//	})
//	if err != nil {
//		return err
//	}
//
//	return center.Scan(ctx, host, port, true, func(ip net.IP, port int) {
//		if portCallback != nil {
//			portCallback(ip.String(), port)
//		}
//	})
//}

func ScanSubDomainQuick(ctx context.Context, target ...string) (chan *subdomain.SubdomainResult, error) {
	config := subdomain.NewSubdomainScannerConfig()
	return scanSubDomain(ctx, config, target...)
}

func scanSubDomain(ctx context.Context, config *subdomain.SubdomainScannerConfig, target ...string) (chan *subdomain.SubdomainResult, error) {
	scanner, err := subdomain.NewSubdomainScanner(
		config, target...,
	)
	if err != nil {
		return nil, err
	}

	var outC = make(chan *subdomain.SubdomainResult)
	scanner.OnResult(func(result *subdomain.SubdomainResult) {
		defer func() {
			if i := recover(); i != nil {
				log.Error(i)
			}
		}()
		select {
		case outC <- result:
		case <-ctx.Done():
		}
	})

	go func() {
		defer close(outC)

		scanner.Feed(target...)
		err = scanner.RunWithContext(ctx)
		if err != nil {
			log.Error(err)
		}
	}()

	return outC, nil
}
