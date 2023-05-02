package subdomain

import (
	"context"
	"fmt"
	"github.com/miekg/dns"
	"github.com/pkg/errors"
	"strings"
	"sync"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func qualifyDomain(domain string) string {
	return fmt.Sprintf("%s.", formatDomain(domain))
}

func formatDomain(target string) string {
	for strings.HasPrefix(target, ".") {
		target = target[1:]
	}
	return target
}

func removeRepeatedMode(modes ...int) []int {
	var ret sync.Map
	for _, m := range modes {
		switch m {
		case BRUTE:
		case SEARCH:
		case ZONE_TRANSFER:
		default:
			continue
		}
		ret.Store(m, 1)
	}

	modes = []int{}
	ret.Range(func(key, value interface{}) bool {
		modes = append(modes, key.(int))
		return true
	})
	return modes
}

func removeRepeatedTargets(targets ...string) []string {
	var ret sync.Map
	for _, m := range targets {
		if strings.TrimSpace(m) != "" {
			ret.Store(m, 1)
		}
	}

	targets = []string{}
	ret.Range(func(key, value interface{}) bool {
		targets = append(targets, key.(string))
		return true
	})
	return targets
}

type SubdomainScanner struct {
	logger *log.Logger

	targets []string
	config  *SubdomainScannerConfig

	dnsQuerierSwg utils.SizedWaitGroup
	dnsClient     *dns.Client

	// 结果回调函数
	resultCallbacks []ResultCallback

	// 解析失败回调
	// 解析失败不是由爆破调用的，这个调用链只会涉及到搜索以及域传送
	resultFailedCallbacks []ResultCallback

	resultCacher *sync.Map
}

func (s *SubdomainScanner) GetConfig() *SubdomainScannerConfig {
	return s.config
}

func NewSubdomainScannerWithLogger(config *SubdomainScannerConfig, logger *log.Logger, targets ...string) (*SubdomainScanner, error) {
	if config.WorkerCount <= 0 {
		config.WorkerCount = 50
	}

	client := &dns.Client{}
	client.Timeout = config.TimeoutForEachQuery

	if logger == nil {
		logger = log.DefaultLogger
	}

	return &SubdomainScanner{
		logger: logger,

		targets: targets,
		config:  config,

		dnsClient:     client,
		dnsQuerierSwg: utils.NewSizedWaitGroup(config.WorkerCount),

		resultCacher: new(sync.Map),
	}, nil
}

func NewSubdomainScanner(config *SubdomainScannerConfig, targets ...string) (*SubdomainScanner, error) {
	return NewSubdomainScannerWithLogger(config, nil, targets...)
}

// 追加目标
func (s *SubdomainScanner) Feed(targets ...string) {
	s.targets = append(s.targets, targets...)
}

func (s *SubdomainScanner) RunWithContext(ctx context.Context) error {
	if len(s.targets) <= 0 {
		return errors.New("empty targets list")
	}

	// 规范化 modes
	modes := removeRepeatedMode(s.config.Modes...)
	if len(modes) <= 0 {
		return errors.New("subdomain scan modes is empty.")
	}

	// 限制目标并发
	swg := utils.NewSizedWaitGroup(s.config.ParallelismTasksCount)
	defer swg.Wait()

	// 规范化 targets
	for _, t := range removeRepeatedTargets(s.targets...) {
		// 这个 swg 用来限制整个目标的并发
		err := swg.AddWithContext(ctx)
		if err != nil || ctx.Err() != nil {
			return err
		}

		go func(target string) {
			defer swg.Done()

			// 在针对特定目标进行子域名检测的时候，应该使用为目标单独生成的带 TimeoutSeconds 的 Context
			ctx, _ := context.WithTimeout(ctx, s.config.TimeoutForEachTarget)

			// 针对不同模式启动 goroutine 并发
			wg := utils.NewSizedWaitGroup(3)
			defer wg.Wait()
			for _, mode := range modes {

				// 使用 AddWithContext 安全取消队列中的任务
				err := wg.AddWithContext(ctx)
				if err != nil {
					return
				}

				log.Debugf("start target: %v mode: %v", target, mode)
				switch mode {
				case BRUTE:
					go func() {
						defer wg.Done()
						s.Brute(ctx, target)
					}()
					continue
				case SEARCH:
					go func() {
						defer wg.Done()

						s.Search(ctx, target)
					}()
				case ZONE_TRANSFER:
					go func() {
						defer wg.Done()

						s.ZoneTransfer(ctx, target)
					}()
				default:
					wg.Done()
				}
			}
		}(t)
	}

	return nil
}

func (s *SubdomainScanner) Run() error {
	return s.RunWithContext(context.Background())
}

// 设置发现子域名的回调
type ResultCallback func(*SubdomainResult)

func (s *SubdomainScanner) OnResult(cb ResultCallback) {
	s.resultCallbacks = append(s.resultCallbacks, cb)
}

func (s *SubdomainScanner) onResult(result *SubdomainResult) {
	// 如果是缓存了的域名结果，就别报告了
	if _, ok := s.resultCacher.LoadOrStore(result.Hash(), result); ok {
		return
	}

	for _, cb := range s.resultCallbacks {
		cb(result)
	}
}

// 设置解析失败子域名的回调函数
func (s *SubdomainScanner) OnResolveFailedResult(cb ResultCallback) {
	s.resultFailedCallbacks = append(s.resultFailedCallbacks, cb)
}

func (s *SubdomainScanner) onResolveFailedResult(result *SubdomainResult) {
	for _, cb := range s.resultFailedCallbacks {
		cb(result)
	}
}
