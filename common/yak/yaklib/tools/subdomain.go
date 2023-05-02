package tools

import (
	"github.com/davecgh/go-spew/spew"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/subdomain"
	"yaklang.io/yaklang/common/utils"
)

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

	var chRes = make(chan *subdomain.SubdomainResult, 10000)
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

var SubDomainExports = map[string]interface{}{
	"Scan": _subdomainScan,

	// 选项
	"wildcardToStop":   subdomain.WithWildCardToStop,
	"recursive":        subdomain.WithAllowToRecursive,
	"workerConcurrent": subdomain.WithWorkerCount,
	"dnsServer":        subdomain.WithDNSServers,
	"maxDepth":         subdomain.WithMaxDepth,
	"targetConcurrent": subdomain.WithParallelismTasksCount,
	"targetTimeout": func(i float64) subdomain.ConfigOption {
		return subdomain.WithTimeoutForEachTarget(utils.FloatSecondDuration(i))
	},
	"eachQueryTimeout": func(i float64) subdomain.ConfigOption {
		return subdomain.WithTimeoutForEachQuery(utils.FloatSecondDuration(i))
	},
	"eachSearchTimeout": func(i float64) subdomain.ConfigOption {
		return subdomain.WithTimeoutForEachHTTPSearch(utils.FloatSecondDuration(i))
	},

	// 自定义主字典
	"mainDict": func(i interface{}) subdomain.ConfigOption {
		return subdomain.WithMainDictionary(utils.StringAsFileParams(i))
	},

	// 递归字典
	"recursiveDict": func(i interface{}) subdomain.ConfigOption {
		return subdomain.WithSubDictionary(utils.StringAsFileParams(i))
	},
}
