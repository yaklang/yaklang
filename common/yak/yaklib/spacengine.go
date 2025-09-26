package yaklib

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine"
)

var SpaceEngineExports = map[string]interface{}{
	"ShodanQuery":  _shodan,
	"FofaQuery":    _fofa,
	"QuakeQuery":   _quake,
	"HunterQuery":  _hunter,
	"ZoomeyeQuery": _zoomeye,

	"Query": Query,

	"domain":      _spaceEngine_Domain,
	"maxPage":     _spaceEngine_MaxPage,
	"maxRecord":   _spaceEngine_MaxRecord,
	"pageSize":    _spaceEngine_PageSize,
	"randomDelay": _spaceEngine_RandomDelay,
	"retryTimes":  _spaceEngine_RetryTimes,
	"zoomeye":     withUseZoomeye,
	"shodan":      withUseShodan,
	"quake":       withUseQuake,
	"hunter":      withUseHunter,
	"fofa":        withUseFofa,
	"engine":      withEngine,
}

func withEngine(i string, auth ...string) _spaceEngineConfigOpt {
	switch strings.TrimSpace(strings.ToLower(i)) {
	case "zoomeye":
		return withUseZoomeye(auth...)
	case "shodan":
		return withUseShodan(auth...)
	case "quake":
		return withUseQuake(auth...)
	case "hunter":
		return withUseHunter(auth...)
	case "fofa":
		return withUseFofa(auth...)
	}
	return func(c *_spaceEngineConfig) {
		defaultConfig := &base.BaseSpaceEngineConfig{}
		if err := consts.GetThirdPartyApplicationConfig(i, defaultConfig); err != nil {
			log.Debug(err)
		} else {
			c.apiKey = defaultConfig.APIKey
			c.user = defaultConfig.UserIdentifier
			c.domain = defaultConfig.Domain
		}
		c.engine = i
		switch len(auth) {
		case 1:
			c.apiKey = auth[0]
		case 2:
			c.user = auth[0]
			c.apiKey = auth[1]
		case 3:
			c.user = auth[0]
			c.apiKey = auth[1]
			c.domain = auth[2]
		}
	}
}

func withUseZoomeye(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "zoomeye"
		cfg := &base.BaseSpaceEngineConfig{}
		err := consts.GetThirdPartyApplicationConfig("zoomeye", cfg)
		if err != nil {
			log.Debug(err)
		}
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = cfg.APIKey
		}
		c.domain = cfg.Domain
	}
}

func withUseShodan(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "shodan"
		cfg := &base.BaseSpaceEngineConfig{}
		err := consts.GetThirdPartyApplicationConfig("shodan", cfg)
		if err != nil {
			log.Debug(err)
		}
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = cfg.APIKey
		}
		c.domain = cfg.Domain
	}
}

func withUseQuake(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "quake"
		cfg := &base.BaseSpaceEngineConfig{}
		err := consts.GetThirdPartyApplicationConfig("quake", cfg)
		if err != nil {
			log.Debug(err)
		}
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = cfg.APIKey
		}
		c.domain = cfg.Domain
	}
}

func withUseHunter(auth ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "hunter"
		cfg := &base.BaseSpaceEngineConfig{}
		err := consts.GetThirdPartyApplicationConfig("hunter", cfg)
		if err != nil {
			log.Debug(err)
		}
		if len(auth) > 0 {
			c.apiKey = auth[0]
		} else {
			c.apiKey = cfg.APIKey
		}
		c.domain = cfg.Domain
	}
}

func withUseFofa(auth ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "fofa"
		cfg := &base.BaseSpaceEngineConfig{}
		err := consts.GetThirdPartyApplicationConfig("fofa", cfg)
		if err != nil {
			log.Debug(err)
		}
		if len(auth) > 1 {
			c.user = auth[0]
			c.apiKey = auth[1]
		} else {
			c.apiKey = cfg.APIKey
			c.user = cfg.UserIdentifier
		}
		c.domain = cfg.Domain
	}
}

func Query(filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}
	for _, op := range opts {
		op(config)
	}
	switch strings.ToLower(config.engine) {
	case "shodan":
		return _shodan(config.apiKey, filter, opts...)
	case "fofa":
		return _fofa(config.user, config.apiKey, filter, opts...)
	case "quake":
		return _quake(config.apiKey, filter, opts...)
	case "hunter":
		return _hunter(config.user, config.apiKey, filter, opts...)
	case "zoomeye":
		return _zoomeye(config.apiKey, filter, opts...)
	default:
		return nil, utils.Error("invalid engine " + config.engine)
	}
}

type _spaceEngineConfig struct {
	engine           string
	apiKey           string
	user             string
	domain           string
	maxRecord        int
	maxPage          int
	pageSize         int
	randomDelayRange int // 随机延迟范围（秒），0 表示无延迟
	retryTimes       int // 重试次数，0 表示不重试
}

type _spaceEngineConfigOpt func(c *_spaceEngineConfig)

func _spaceEngine_Domain(domain string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.domain = domain
	}
}

func _spaceEngine_MaxRecord(i int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.maxRecord = i
	}
}

func _spaceEngine_MaxPage(i int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.maxPage = i
	}
}

func _spaceEngine_PageSize(i int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.pageSize = i
	}
}

func _spaceEngine_RandomDelay(delayRange int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.randomDelayRange = delayRange
	}
}

func _spaceEngine_RetryTimes(times int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.retryTimes = times
	}
}

func _shodan(token string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord:        1000, // 默认最多1000条记录
		maxPage:          10,   // 默认最多翻10页
		pageSize:         100,  // 每页100条
		randomDelayRange: 0,
		retryTimes:       0,
	}

	for _, opt := range opts {
		opt(config)
	}

	queryConfig := &base.QueryConfig{
		RandomDelayRange: config.randomDelayRange,
		RetryTimes:       config.retryTimes,
	}
	return spacengine.ShodanQueryWithConfig(token, filter, config.maxPage, config.maxRecord, queryConfig, config.domain)
}

func _quake(token string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord:        1000, // 默认最多1000条记录
		maxPage:          10,   // 默认最多翻10页
		pageSize:         100,  // 每页100条
		randomDelayRange: 0,
		retryTimes:       0,
	}

	for _, opt := range opts {
		opt(config)
	}

	queryConfig := &base.QueryConfig{
		RandomDelayRange: config.randomDelayRange,
		RetryTimes:       config.retryTimes,
	}
	return spacengine.QuakeQueryWithConfig(token, filter, config.maxPage, config.maxRecord, queryConfig, config.domain)
}

func _hunter(name, key string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord:        1000, // 默认最多1000条记录
		maxPage:          10,   // 默认最多翻10页
		pageSize:         10,   // Hunter限制每页最多10条
		randomDelayRange: 0,
		retryTimes:       0,
	}

	for _, opt := range opts {
		opt(config)
	}

	queryConfig := &base.QueryConfig{
		RandomDelayRange: config.randomDelayRange,
		RetryTimes:       config.retryTimes,
	}
	return spacengine.HunterQueryWithConfig(key, filter, config.maxPage, config.pageSize, config.maxRecord, queryConfig, config.domain)
}

func _fofa(email, key string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord:        1000, // 默认最多1000条记录
		maxPage:          10,   // 默认最多翻10页
		pageSize:         100,  // 每页100条
		randomDelayRange: 0,
		retryTimes:       0,
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.pageSize > 10000 {
		log.Warn("fofa page size maximum 10000")
		config.pageSize = 10000
	}

	queryConfig := &base.QueryConfig{
		RandomDelayRange: config.randomDelayRange,
		RetryTimes:       config.retryTimes,
	}
	return spacengine.FofaQueryWithConfig(email, key, filter, config.maxPage, config.pageSize, config.maxRecord, queryConfig, config.domain)
}

func _zoomeye(key string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord:        1000, // 默认最多1000条记录
		maxPage:          10,   // 默认最多翻10页
		pageSize:         100,  // 每页100条
		randomDelayRange: 0,
		retryTimes:       0,
	}

	for _, opt := range opts {
		opt(config)
	}

	queryConfig := &base.QueryConfig{
		RandomDelayRange: config.randomDelayRange,
		RetryTimes:       config.retryTimes,
	}
	return spacengine.ZoomeyeQueryWithConfig(key, filter, config.maxPage, config.maxRecord, queryConfig, config.domain)
}
