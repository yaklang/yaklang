package yaklib

import (
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
}

type _spaceEngineConfig struct {
	maxRecord int
	maxPage   int
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

	return spacengine2.HunterQuery(name, key, filter, config.maxPage, config.maxRecord)
}

func _fofa(email, key string, filter string, opts ..._spaceEngineConfigOpt) (chan *spacengine2.NetSpaceEngineResult, error) {
	config := &_spaceEngineConfig{
		maxRecord: 100,
		maxPage:   10,
	}

	for _, opt := range opts {
		opt(config)
	}

	return spacengine2.FofaQuery(email, key, filter, config.maxPage, config.maxRecord)
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
