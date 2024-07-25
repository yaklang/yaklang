package tools

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/subdomain"
	"github.com/yaklang/yaklang/common/utils"
)

// Scan 对域名进行子域名扫描，它的第一个参数可以接收字符串或字符串数组，接下来可以接收零个到多个选项，用于对此次扫描进行配置，例如设置扫描超时时间，是否递归等，返回结果管道与错误
// 使用 请求(爆破)，查询，域传送技术进行子域名扫描
// Example:
// ```
// for domain in subdomain.Scan("example.com")~ {
// dump(domain)
// }
// ```
func _subdomainScan(target interface{}, opts ...subdomain.ConfigOption) (chan *subdomain.SubdomainResult, error) {
	var targets []string
	switch ret := target.(type) {
	case string:
		targets = utils.ParseStringToHosts(ret)
	case []byte:
		targets = append(targets, utils.ParseStringToHosts(string(ret))...)
	case []string:
		targets = ret
	default:
		return nil, utils.Errorf("unsupported target: %v", spew.Sdump(target))
	}

	config := subdomain.NewSubdomainScannerConfig()
	for _, opt := range opts {
		opt(config)
	}

	scanner, err := subdomain.NewSubdomainScanner(config, targets...)
	if err != nil {
		return nil, err
	}

	chRes := make(chan *subdomain.SubdomainResult, 10000)
	scanner.OnResult(func(result *subdomain.SubdomainResult) {
		defer func() {
			if err := recover(); err != nil {
				log.Warningf("subdomain output result error: %v", err)
				return
			}
		}()
		select {
		case chRes <- result:
		}
	})

	go func() {
		defer close(chRes)
		err := scanner.Run()
		if err != nil {
			log.Errorf("subdomain instance run[%v] failed: %s", targets, err)
		}
	}()

	return chRes, nil
}

// targetTimeout 是一个选项参数，设置每个目标的超时时间，单位为秒，默认为 300s
// Example:
// ```
// subdomain.Scan("example.com", subdomain.targetTimeout(10))
// ```
func withTargetTimeout(i float64) subdomain.ConfigOption {
	return subdomain.WithTimeoutForEachTarget(utils.FloatSecondDuration(i))
}

// eachQueryTimeout 是一个选项参数，设置每个查询的超时时间，单位为秒，默认为 3s
// Example:
// ```
// subdomain.Scan("example.com", subdomain.eachQueryTimeout(5))
// ```
func withEachQueryTimeout(i float64) subdomain.ConfigOption {
	return subdomain.WithTimeoutForEachQuery(utils.FloatSecondDuration(i))
}

// withEachSearchTimeout 是一个选项参数，设置每个搜索的超时时间，单位为秒，默认为 10s
// Example:
// ```
// subdomain.Scan("example.com", subdomain.withEachSearchTimeout(5))
// ```
func withEachSearchTimeout(i float64) subdomain.ConfigOption {
	return subdomain.WithTimeoutForEachHTTPSearch(utils.FloatSecondDuration(i))
}

// mainDict 是一个选项参数，设置子域名爆破主字典，其第一个参数可以是文件名、字符串或字符串数组
// Example:
// ```
// dict = "/tmp/dict.txt"
// subdomain.Scan("example.com", subdomain.mainDict(dict))
// ```
func withMainDict(i any) subdomain.ConfigOption {
	return subdomain.WithMainDictionary(utils.StringAsFileParams(i))
}

// recursiveDict 是一个选项参数，设置子域名爆破递归字典，其第一个参数可以是文件名、字符串或字符串数组
// Example:
// ```
// dict = "/tmp/sub-dict.txt"
// subdomain.Scan("example.com", subdomain.recursive(true), subdomain.recursiveDict(dict))
// ```
func withRecursiveDict(i any) subdomain.ConfigOption {
	return subdomain.WithSubDictionary(utils.StringAsFileParams(i))
}

var SubDomainExports = map[string]interface{}{
	"Scan": _subdomainScan,

	// 选项
	"wildcardToStop":    subdomain.WithWildCardToStop,
	"recursive":         subdomain.WithAllowToRecursive,
	"workerConcurrent":  subdomain.WithWorkerCount,
	"dnsServer":         subdomain.WithDNSServers,
	"maxDepth":          subdomain.WithMaxDepth,
	"targetConcurrent":  subdomain.WithParallelismTasksCount,
	"targetTimeout":     withTargetTimeout,
	"eachQueryTimeout":  withEachQueryTimeout,
	"eachSearchTimeout": withEachSearchTimeout,

	"mainDict":      withMainDict,
	"recursiveDict": withRecursiveDict,
}
