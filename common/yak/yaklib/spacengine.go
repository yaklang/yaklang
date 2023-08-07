package yaklib

import (
	"github.com/yaklang/yaklang/common/log"
	spacengine2 "github.com/yaklang/yaklang/common/utils/spacengine"
)

var SpaceEngineExports = map[string]interface{}{
	"ShodanQuery":  _shodan,
	"FofaQuery":    _fofa,
	"QuakeQuery":   _quake,
	"HunterQuery":  _hunter,
	"ZoomeyeQuery": _zoomeye,

	"maxPage":   _spaceEngine_MaxPage,
	"maxRecord": _spaceEngine_MaxRecord,
	"pageSize":  _spaceEngine_PageSize,
}

type _spaceEngineConfig struct {
	maxRecord int
	maxPage   int
	pageSize  int
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
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine2.ZoomeyeQuery(key, filter, config.maxPage, config.maxRecord)
}
