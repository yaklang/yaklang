package yaklib

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	spacengine2 "github.com/yaklang/yaklang/common/utils/spacengine"
	"strings"
)

var SpaceEngineExports = map[string]interface{}{
	"ShodanQuery":  _shodan,
	"FofaQuery":    _fofa,
	"QuakeQuery":   _quake,
	"HunterQuery":  _hunter,
	"ZoomeyeQuery": _zoomeye,

	"Query": Query,

	"maxPage":   _spaceEngine_MaxPage,
	"maxRecord": _spaceEngine_MaxRecord,
	"pageSize":  _spaceEngine_PageSize,
	"zoomeye":   withUseZoomeye,
	"shodan":    withUseShodan,
	"quake":     withUseQuake,
	"hunter":    withUseHunter,
	"fofa":      withUseFofa,
	"engine":    withEngine,
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
		c.engine = i
		if len(auth) == 1 {
			c.apiKey = auth[0]
		} else {
			c.apiKey = consts.GetThirdPartyApplicationConfig(i).APIKey
		}
		if len(auth) == 2 {
			c.user = auth[0]
			c.apiKey = auth[1]
		} else {
			c.apiKey = consts.GetThirdPartyApplicationConfig(i).APIKey
			c.user = consts.GetThirdPartyApplicationConfig(i).UserIdentifier
		}
	}
}

func withUseZoomeye(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "zoomeye"
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = consts.GetThirdPartyApplicationConfig("zoomeye").APIKey
		}
	}
}

func withUseShodan(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "shodan"
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = consts.GetThirdPartyApplicationConfig("shodan").APIKey
		}
	}
}

func withUseQuake(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "quake"
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = consts.GetThirdPartyApplicationConfig("quake").APIKey
		}
	}
}

func withUseHunter(auth ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "hunter"
		if len(auth) > 0 {
			c.apiKey = auth[0]
		} else {
			c.apiKey = consts.GetThirdPartyApplicationConfig("hunter").APIKey
		}
	}
}

func withUseFofa(auth ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "fofa"
		if len(auth) > 1 {
			c.user = auth[0]
			c.apiKey = auth[1]
		} else {
			c.apiKey = consts.GetThirdPartyApplicationConfig("fofa").APIKey
			c.user = consts.GetThirdPartyApplicationConfig("fofa").UserIdentifier
		}
	}
}

func Query(filter string, opts ..._spaceEngineConfigOpt) (chan *spacengine2.NetSpaceEngineResult, error) {
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
	maxRecord int
	maxPage   int
	pageSize  int
	engine    string
	apiKey    string
	user      string
}

type _spaceEngineConfigOpt func(c *_spaceEngineConfig)

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

func _shodan(token string, filter string, opts ..._spaceEngineConfigOpt) (chan *spacengine2.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine2.ShodanQuery(token, filter, config.maxPage, config.maxRecord)
}

func _quake(token string, filter string, opts ..._spaceEngineConfigOpt) (chan *spacengine2.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine2.QuakeQuery(token, filter, config.maxPage, config.maxRecord)
}

func _hunter(name, key string, filter string, opts ..._spaceEngineConfigOpt) (chan *spacengine2.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine2.HunterQuery(name, key, filter, config.maxPage, config.pageSize, config.maxRecord)
}

func _fofa(email, key string, filter string, opts ..._spaceEngineConfigOpt) (chan *spacengine2.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  100,
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.pageSize > 10000 {
		log.Warn("fofa page size maximum 10000")
		config.pageSize = 10000
	}

	return spacengine2.FofaQuery(email, key, filter, config.maxPage, config.pageSize, config.maxRecord)
}

func _zoomeye(key string, filter string, opts ..._spaceEngineConfigOpt) (chan *spacengine2.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine2.ZoomeyeQuery(key, filter, config.maxPage, config.maxRecord)
}
