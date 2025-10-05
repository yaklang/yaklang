package aicommon

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type KeyValueConfigIf interface {
	HaveConfig(string) bool
	GetConfig(string) (any, bool)
	GetConfigString(string, ...string) string
	GetConfigInt(string, ...int) int
	GetConfigInt64(string, ...int64) int64
	GetConfigFloat64(string, ...float64) float64
	GetConfigBool(string, ...bool) bool
	SetConfig(string, any)
}

var _ KeyValueConfigIf = (*KeyValueConfig)(nil)

type KeyValueConfig struct {
	vars *omap.OrderedMap[string, any]
}

func NewKeyValueConfig() *KeyValueConfig {
	return &KeyValueConfig{
		vars: omap.NewOrderedMap(make(map[string]any)),
	}
}

func (r *KeyValueConfig) HaveConfig(key string) bool {
	return r.vars.Have(key)
}

func (r *KeyValueConfig) GetConfig(key string) (any, bool) {
	if r.HaveConfig(key) {
		return r.vars.Get(key)
	}
	return nil, false
}

func (r *KeyValueConfig) GetConfigString(key string, defaults ...string) string {
	val, ok := r.GetConfig(key)
	if ok {
		return utils.InterfaceToString(val)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return ""
}

func (r *KeyValueConfig) GetConfigInt(key string, defaults ...int) int {
	val, ok := r.GetConfig(key)
	if ok {
		return utils.InterfaceToInt(val)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return 0
}

func (r *KeyValueConfig) GetConfigBool(key string, defaults ...bool) bool {
	val, ok := r.GetConfig(key)
	if ok {
		return utils.InterfaceToBoolean(val)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return false
}

func (r *KeyValueConfig) SetConfig(key string, value any) {
	r.vars.Set(key, value)
}

func (r *KeyValueConfig) GetConfigInt64(key string, defaults ...int64) int64 {
	var results = make([]int, len(defaults))
	for i, v := range defaults {
		results[i] = int(v)
	}
	return int64(r.GetConfigInt(key, results...))
}

func (r *KeyValueConfig) GetConfigFloat64(key string, defaults ...float64) float64 {
	val, ok := r.GetConfig(key)
	if ok {
		return utils.InterfaceToFloat64(val)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return 0.0
}
