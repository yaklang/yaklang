package bin_parser

import (
	"errors"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"strconv"
	"strings"
)

type Config struct {
	endian        binx.ByteOrderEnum // 子节点会自动继承

	dataType      binx.BinaryTypeVerbose
	refDataType   string // 引用的数据类型
	hasHalf       bool   // 半字节
	length        uint64 // 两种含义：isList时表示数组长度，否则表示字节长度
	autoLength    bool
	getAutoLength func() (uint64, error)
	isList        bool
	total         any
}

func NewConfig(opts []ConfigFunc) *Config {
	cfg := &Config{
		endian:   binx.BigEndianByteOrder,
		dataType: "",
		length:   0,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

type ConfigFunc func(config *Config)

func WithList(b bool) ConfigFunc {
	return func(config *Config) {
		config.isList = b
	}
}
func WithEndian(e binx.ByteOrderEnum) ConfigFunc {
	return func(config *Config) {
		config.endian = e
	}
}
func WithDataType(s binx.BinaryTypeVerbose) ConfigFunc {
	return func(config *Config) {
		config.dataType = s
	}
}
func WithHasHalf(b bool) ConfigFunc {
	return func(config *Config) {
		config.hasHalf = b
	}
}
func WithTotal(t any) ConfigFunc {
	return func(config *Config) {
		config.total = t
	}
}

func WithLength(l uint64) ConfigFunc {
	return func(config *Config) {
		config.length = l
	}
}
func WithAutoLength(b bool) ConfigFunc {
	return func(config *Config) {
		config.autoLength = b
	}
}
func parseTerminalNodeWithConfig(raw string, node *Node) ([]ConfigFunc, error) {
	splits := strings.Split(strings.ToLower(raw), ",")
	if len(splits) == 0 {
		return nil, errors.New("invalid type: " + raw)
	}
	parse := func(params [3]string) ([]ConfigFunc, error) { // list, type, length
		options := []ConfigFunc{}
		if params[0] == "list" {
			options = append(options, WithList(true))
		}
		if !utils.StringArrayContains([]string{"int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "raw", "string"}, params[1]) {
			if node != nil && node.root != nil {
				//,node.root.Get(params[1])
			}
			return nil, errors.New("invalid type: " + params[1])
		}
		options = append(options, WithDataType(binx.BinaryTypeVerbose(params[1])))
		n, err := strconv.ParseUint(params[2], 10, 64)
		if err != nil {
			n, err := strconv.ParseFloat(params[2], 64)
			if err != nil {
				return nil, errors.New("invalid length: " + params[2])
			}
			integerN := n - 0.5
			if float64(uint64(integerN)) != integerN {
				return nil, errors.New("invalid length: " + params[2])
			}
			options = append(options, WithHasHalf(true))
			options = append(options, WithLength(uint64(integerN)))
		} else {
			options = append(options, WithLength(n))
		}
		return options, nil
	}
	switch len(splits) {
	case 3:
		return parse([3]string{splits[0], splits[1], splits[2]})
	case 2:
		if splits[0] == "list" {
			return parse([3]string{splits[0], splits[1], ""})
		} else {
			return parse([3]string{"", splits[0], splits[1]})
		}
	case 1:
		var length uint64 = 0
		switch splits[0] {
		case "int":
			length = 4
		case "uint":
			length = 4
		case "int8":
			length = 1
		case "uint8":
			length = 1
		case "int16":
			length = 2
		case "uint16":
			length = 2
		case "int32":
			length = 4
		case "uint32":
			length = 4
		case "int64":
			length = 8
		case "uint64":
			length = 8
		case "raw":
			if node.cfg.total == nil {
				return nil, errors.New("auto raw type must have total")
			}
			totalStr := utils.InterfaceToString(node.cfg.total)
			n, err := strconv.ParseUint(totalStr, 10, 64)
			if err == nil {
				length = n
			} else {
				n, err := node.root.Get(totalStr)
				if err != nil {
					return nil, err
				}
				switch ret := n.result.(type) {
				case int:
					length = uint64(ret)
				case int8:
					length = uint64(ret)
				case int16:
					length = uint64(ret)
				case int32:
					length = uint64(ret)
				case int64:
					length = uint64(ret)
				case uint:
					length = uint64(ret)
				case uint8:
					length = uint64(ret)
				case uint16:
					length = uint64(ret)
				case uint32:
					length = uint64(ret)
				case uint64:
					length = uint64(ret)
				default:
					return nil, errors.New("invalid total type: " + utils.InterfaceToString(n.result))
				}
			}
		default:
			return nil, errors.New("invalid type: " + splits[0])
		}
		return parse([3]string{"", splits[0], strconv.FormatUint(length, 10)})
	default:
		return nil, errors.New("invalid type: " + raw)
	}
}
func parseTerminalNode(raw string) ([]ConfigFunc, error) {
	return parseTerminalNodeWithConfig(raw, nil)
}
func yamlMapToGoMap(d yaml.MapSlice) map[any]any {
	m := map[any]any{}
	for _, v := range d {
		switch v.Value.(type) {
		case yaml.MapSlice:
			m[v.Key] = yamlMapToGoMap(v.Value.(yaml.MapSlice))
		default:
			m[v.Key] = v.Value
		}
	}
	return m
}

// getNode 从node中提取配置信息
func splitConfigAndData(d any) ([]ConfigFunc, any) {
	defaultConfig := []ConfigFunc{}
	var getConfigFromSlice func(d any) ([]ConfigFunc, any)
	getConfigFromSlice = func(d any) ([]ConfigFunc, any) {
		switch ret := d.(type) {
		case yaml.MapSlice:
			newRes := yaml.MapSlice{}
			data := ""
			for i, item := range ret {
				if item.Key == "config" {
					config, _ := getConfigFromSlice(ret[0].Value)
					return config, append(ret[:i], ret[i+1:]...)
				}
				switch item.Key {
				case "endian":
					if strings.ToLower(utils.InterfaceToString(item.Value)) == "big" {
						defaultConfig = append(defaultConfig, WithEndian(binx.BigEndianByteOrder))
					} else if strings.ToLower(utils.InterfaceToString(item.Value)) == "little" {
						defaultConfig = append(defaultConfig, WithEndian(binx.LittleEndianByteOrder))
					} else {
						defaultConfig = append(defaultConfig, WithEndian(binx.BigEndianByteOrder))
					}
				case "type":
					defaultConfig = append(defaultConfig, WithDataType(binx.BinaryTypeVerbose(utils.InterfaceToString(item.Value))))
				case "length":
					n, err := strconv.ParseUint(utils.InterfaceToString(item.Value), 10, 64)
					if err != nil {
						log.Errorf("parse length error: %v", err)
					} else {
						defaultConfig = append(defaultConfig, WithLength(n))
					}
				case "data":
					data = utils.InterfaceToString(item.Value)
				case "total":
					defaultConfig = append(defaultConfig, WithTotal(item.Value))
				default:
					newRes = append(newRes, item)
				}
			}
			if data != "" {
				return defaultConfig, data
			}
			return defaultConfig, newRes
		}
		return defaultConfig, d
	}
	cfg, node := getConfigFromSlice(d)
	if v, ok := node.(yaml.MapSlice); ok && len(v) == 1 && v[0].Key == "Data" {
		return cfg, v[0]
	}
	return cfg, node
}
