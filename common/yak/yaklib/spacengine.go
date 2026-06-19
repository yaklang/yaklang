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
	"ZoneQuery":    _zone,

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
	"zone":        withUseZone,
	"engine":      withEngine,
}

// engine 选择要使用的网络空间测绘引擎并设置鉴权信息，支持 zoomeye/shodan/quake/hunter/fofa/zone 及自定义
// 在 yak 中通过 spacengine.engine 调用，未显式传入鉴权时会尝试读取本地第三方应用配置
// 参数:
//   - i: 引擎名称
//   - auth: 可选鉴权参数(依引擎不同为 apiKey，或 user+apiKey，或 user+apiKey+domain)
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：选择 fofa 引擎并传入鉴权
// res = spacengine.Query("app=\"nginx\"", spacengine.engine("fofa", "user@example.com", "APIKEY"))~
// ```
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
	case "zone":
		return withUseZone(auth...)
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

// zoomeye 选择使用 ZoomEye 引擎并设置 API Key
// 在 yak 中通过 spacengine.zoomeye 调用，未传入时尝试读取本地配置
// 参数:
//   - api: 可选的 ZoomEye API Key
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用 ZoomEye 引擎
// res = spacengine.Query("nginx", spacengine.zoomeye("YOUR_API_KEY"))~
// ```
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

// shodan 选择使用 Shodan 引擎并设置 API Key
// 在 yak 中通过 spacengine.shodan 调用，未传入时尝试读取本地配置
// 参数:
//   - api: 可选的 Shodan API Key
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用 Shodan 引擎
// res = spacengine.Query("apache", spacengine.shodan("YOUR_API_KEY"))~
// ```
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

// quake 选择使用 Quake(360) 引擎并设置 API Key
// 在 yak 中通过 spacengine.quake 调用，未传入时尝试读取本地配置
// 参数:
//   - api: 可选的 Quake API Key
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用 Quake 引擎
// res = spacengine.Query("nginx", spacengine.quake("YOUR_API_KEY"))~
// ```
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

// hunter 选择使用 Hunter(奇安信) 引擎并设置 API Key
// 在 yak 中通过 spacengine.hunter 调用，未传入时尝试读取本地配置
// 参数:
//   - auth: 可选的 Hunter API Key
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用 Hunter 引擎
// res = spacengine.Query("nginx", spacengine.hunter("YOUR_API_KEY"))~
// ```
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

// fofa 选择使用 FOFA(白帽汇) 引擎并设置鉴权(邮箱与 API Key)
// 在 yak 中通过 spacengine.fofa 调用，未传入时尝试读取本地配置
// 参数:
//   - auth: 可选鉴权参数，传两个时为 邮箱 与 API Key
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用 FOFA 引擎
// res = spacengine.Query("app=\"nginx\"", spacengine.fofa("user@example.com", "YOUR_API_KEY"))~
// ```
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

// zone 选择使用 0.zone 引擎并设置 API Key
// 在 yak 中通过 spacengine.zone 调用，未传入时尝试读取本地配置
// 参数:
//   - api: 可选的 0.zone API Key
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用 0.zone 引擎
// res = spacengine.Query("nginx", spacengine.zone("YOUR_API_KEY"))~
// ```
func withUseZone(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "zone"
		cfg := &base.BaseSpaceEngineConfig{}
		err := consts.GetThirdPartyApplicationConfig("zone", cfg)
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

// Query 使用所选网络空间测绘引擎执行查询，以 channel 形式流式返回结果
// 在 yak 中通过 spacengine.Query 调用，需通过 spacengine.engine 等选项指定引擎与鉴权
// 参数:
//   - filter: 查询语句(语法依各引擎而定)
//   - opts: 配置项，需含引擎选择(如 spacengine.fofa)，以及 spacengine.maxRecord 等
//
// 返回值:
//   - 一个只读 channel，逐条产出测绘结果
//   - 错误信息，引擎无效或查询失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部测绘服务与有效 API Key
// res = spacengine.Query("app=\"nginx\"",
//
//	spacengine.fofa("user@example.com", "YOUR_API_KEY"),
//	spacengine.maxRecord(100),
//
// )~
//
//	for result = range res {
//	    println(result.Addr)
//	}
//
// ```
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
	case "zone":
		return _zone(config.apiKey, filter, opts...)
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

// domain 设置测绘引擎的自定义 API 域名/Endpoint(适配私有化部署)
// 在 yak 中通过 spacengine.domain 调用
// 参数:
//   - domain: 自定义 API 域名
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定自定义 API 域名
// res = spacengine.Query("nginx", spacengine.fofa("u", "k"), spacengine.domain("fofa.example.com"))~
// ```
func _spaceEngine_Domain(domain string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.domain = domain
	}
}

// maxRecord 设置本次查询返回的最大记录数
// 在 yak 中通过 spacengine.maxRecord 调用
// 参数:
//   - i: 最大记录数
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：最多返回 100 条记录
// res = spacengine.Query("nginx", spacengine.fofa("u", "k"), spacengine.maxRecord(100))~
// ```
func _spaceEngine_MaxRecord(i int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.maxRecord = i
	}
}

// maxPage 设置本次查询最多翻页的页数
// 在 yak 中通过 spacengine.maxPage 调用
// 参数:
//   - i: 最大页数
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：最多翻 5 页
// res = spacengine.Query("nginx", spacengine.fofa("u", "k"), spacengine.maxPage(5))~
// ```
func _spaceEngine_MaxPage(i int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.maxPage = i
	}
}

// pageSize 设置每页返回的记录数(各引擎有不同上限)
// 在 yak 中通过 spacengine.pageSize 调用
// 参数:
//   - i: 每页记录数
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：每页 100 条
// res = spacengine.Query("nginx", spacengine.fofa("u", "k"), spacengine.pageSize(100))~
// ```
func _spaceEngine_PageSize(i int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.pageSize = i
	}
}

// randomDelay 设置每次翻页请求之间的随机延迟范围(秒)，用于规避频率限制
// 在 yak 中通过 spacengine.randomDelay 调用
// 参数:
//   - delayRange: 随机延迟范围(秒)，0 表示无延迟
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：每页间随机延迟 0-3 秒
// res = spacengine.Query("nginx", spacengine.fofa("u", "k"), spacengine.randomDelay(3))~
// ```
func _spaceEngine_RandomDelay(delayRange int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.randomDelayRange = delayRange
	}
}

// retryTimes 设置请求失败时的重试次数
// 在 yak 中通过 spacengine.retryTimes 调用
// 参数:
//   - times: 重试次数，0 表示不重试
//
// 返回值:
//   - 一个 spacengine.Query 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：失败重试 3 次
// res = spacengine.Query("nginx", spacengine.fofa("u", "k"), spacengine.retryTimes(3))~
// ```
func _spaceEngine_RetryTimes(times int) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.retryTimes = times
	}
}

// ShodanQuery 使用 Shodan 引擎执行查询，以 channel 形式流式返回结果
// 在 yak 中通过 spacengine.ShodanQuery 调用，依赖有效的 Shodan API Key
// 参数:
//   - token: Shodan API Key
//   - filter: 查询语句
//   - opts: 可选配置项，如 spacengine.maxRecord、spacengine.maxPage
//
// 返回值:
//   - 一个只读 channel，逐条产出测绘结果
//   - 错误信息，查询失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部 Shodan 服务
// res = spacengine.ShodanQuery("YOUR_API_KEY", "apache", spacengine.maxRecord(50))~
//
//	for result = range res {
//	    println(result.Addr)
//	}
//
// ```
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

// QuakeQuery 使用 Quake(360) 引擎执行查询，以 channel 形式流式返回结果
// 在 yak 中通过 spacengine.QuakeQuery 调用，依赖有效的 Quake API Key
// 参数:
//   - token: Quake API Key
//   - filter: 查询语句
//   - opts: 可选配置项，如 spacengine.maxRecord、spacengine.maxPage
//
// 返回值:
//   - 一个只读 channel，逐条产出测绘结果
//   - 错误信息，查询失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部 Quake 服务
// res = spacengine.QuakeQuery("YOUR_API_KEY", "nginx", spacengine.maxRecord(50))~
//
//	for result = range res {
//	    println(result.Addr)
//	}
//
// ```
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

// HunterQuery 使用 Hunter(奇安信) 引擎执行查询，以 channel 形式流式返回结果
// 在 yak 中通过 spacengine.HunterQuery 调用，依赖有效的 Hunter 鉴权
// 参数:
//   - name: 用户标识(部分场景使用)
//   - key: Hunter API Key
//   - filter: 查询语句
//   - opts: 可选配置项，如 spacengine.maxRecord
//
// 返回值:
//   - 一个只读 channel，逐条产出测绘结果
//   - 错误信息，查询失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部 Hunter 服务
// res = spacengine.HunterQuery("", "YOUR_API_KEY", "nginx", spacengine.maxRecord(50))~
//
//	for result = range res {
//	    println(result.Addr)
//	}
//
// ```
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

// FofaQuery 使用 FOFA(白帽汇) 引擎执行查询，以 channel 形式流式返回结果
// 在 yak 中通过 spacengine.FofaQuery 调用，依赖有效的 FOFA 邮箱与 API Key
// 参数:
//   - email: FOFA 账号邮箱
//   - key: FOFA API Key
//   - filter: 查询语句(FOFA 语法)
//   - opts: 可选配置项，如 spacengine.maxRecord、spacengine.pageSize
//
// 返回值:
//   - 一个只读 channel，逐条产出测绘结果
//   - 错误信息，查询失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部 FOFA 服务
// res = spacengine.FofaQuery("user@example.com", "YOUR_API_KEY", "app=\"nginx\"", spacengine.maxRecord(50))~
//
//	for result = range res {
//	    println(result.Addr)
//	}
//
// ```
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

// ZoomeyeQuery 使用 ZoomEye(知道创宇) 引擎执行查询，以 channel 形式流式返回结果
// 在 yak 中通过 spacengine.ZoomeyeQuery 调用，依赖有效的 ZoomEye API Key
// 参数:
//   - key: ZoomEye API Key
//   - filter: 查询语句
//   - opts: 可选配置项，如 spacengine.maxRecord
//
// 返回值:
//   - 一个只读 channel，逐条产出测绘结果
//   - 错误信息，查询失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部 ZoomEye 服务
// res = spacengine.ZoomeyeQuery("YOUR_API_KEY", "nginx", spacengine.maxRecord(50))~
//
//	for result = range res {
//	    println(result.Addr)
//	}
//
// ```
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

// ZoneQuery 使用 0.zone 引擎执行查询，以 channel 形式流式返回结果
// 在 yak 中通过 spacengine.ZoneQuery 调用，依赖有效的 0.zone API Key
// 参数:
//   - key: 0.zone API Key
//   - filter: 查询语句
//   - opts: 可选配置项，如 spacengine.maxRecord
//
// 返回值:
//   - 一个只读 channel，逐条产出测绘结果
//   - 错误信息，查询失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：依赖外部 0.zone 服务
// res = spacengine.ZoneQuery("YOUR_API_KEY", "nginx", spacengine.maxRecord(50))~
//
//	for result = range res {
//	    println(result.Addr)
//	}
//
// ```
func _zone(key string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord:        1000, // 默认最多1000条记录
		maxPage:          10,   // 默认最多翻10页
		pageSize:         40,   // 0.zone 每页最大40条
		randomDelayRange: 2,    // 0.zone 建议2秒延迟避免限频
		retryTimes:       0,
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.pageSize > 40 {
		log.Warn("zone page size maximum 40")
		config.pageSize = 40
	}

	queryConfig := &base.QueryConfig{
		RandomDelayRange: config.randomDelayRange,
		RetryTimes:       config.retryTimes,
	}
	return spacengine.ZoneQueryWithConfig(key, filter, config.maxPage, config.maxRecord, queryConfig, config.domain)
}
