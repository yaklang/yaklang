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

	"domain":    _spaceEngine_Domain,
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
			config := consts.GetThirdPartyApplicationConfig(i)
			c.apiKey = config.APIKey
			c.user = config.UserIdentifier
		}
		if len(auth) == 3 {
			c.user = auth[0]
			c.apiKey = auth[1]
			c.domain = auth[2]
		} else {
			config := consts.GetThirdPartyApplicationConfig(i)
			c.apiKey = config.APIKey
			c.user = config.UserIdentifier
			c.domain = config.GetExtraParam("domain")
		}
	}
}

func withUseZoomeye(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "zoomeye"
		config := consts.GetThirdPartyApplicationConfig("zoomeye")
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = config.APIKey
		}
		c.domain = config.GetExtraParam("domain")
	}
}

func withUseShodan(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "shodan"
		config := consts.GetThirdPartyApplicationConfig("shodan")
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = config.APIKey
		}
		c.domain = config.GetExtraParam("domain")
	}
}

func withUseQuake(api ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "quake"
		config := consts.GetThirdPartyApplicationConfig("quake")
		if len(api) > 0 {
			c.apiKey = api[0]
		} else {
			c.apiKey = config.APIKey
		}
		c.domain = config.GetExtraParam("domain")
	}
}

func withUseHunter(auth ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "hunter"
		config := consts.GetThirdPartyApplicationConfig("hunter")
		if len(auth) > 0 {
			c.apiKey = auth[0]
		} else {
			c.apiKey = config.APIKey
		}
		c.domain = config.GetExtraParam("domain")
	}
}

func withUseFofa(auth ...string) _spaceEngineConfigOpt {
	return func(c *_spaceEngineConfig) {
		c.engine = "fofa"
		config := consts.GetThirdPartyApplicationConfig("fofa")
		if len(auth) > 1 {
			c.user = auth[0]
			c.apiKey = auth[1]
		} else {
			c.apiKey = config.APIKey
			c.user = config.UserIdentifier
		}
		c.domain = config.GetExtraParam("domain")
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
	engine    string
	apiKey    string
	user      string
	domain    string
	maxRecord int
	maxPage   int
	pageSize  int
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

func _shodan(token string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine.ShodanQuery(token, filter, config.maxPage, config.maxRecord, config.domain)
}

func _quake(token string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine.QuakeQuery(token, filter, config.maxPage, config.maxRecord, config.domain)
}

func _hunter(name, key string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine.HunterQuery(key, filter, config.maxPage, config.pageSize, config.maxRecord, config.domain)
}

func _fofa(email, key string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
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

	return spacengine.FofaQuery(email, key, filter, config.maxPage, config.pageSize, config.maxRecord, config.domain)
}

func _zoomeye(key string, filter string, opts ..._spaceEngineConfigOpt) (chan *base.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
		pageSize:  10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine.ZoomeyeQuery(key, filter, config.maxPage, config.maxRecord, config.domain)
}
