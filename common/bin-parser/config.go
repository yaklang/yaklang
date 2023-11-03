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
	endian   binx.ByteOrderEnum
	dataType binx.BinaryTypeVerbose
	length   uint64
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
func WithLength(l uint64) ConfigFunc {
	return func(config *Config) {
		config.length = l
	}
}
func parseTerminalNode(raw string) ([]ConfigFunc, error) {
	splits := strings.Split(strings.ToLower(raw), ",")
	options := []ConfigFunc{}
	switch len(splits) {
	case 2:
		if !utils.StringArrayContains([]string{"int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "raw", "string"}, splits[0]) {
			return nil, errors.New("invalid type: " + splits[0])
		}
		options = append(options, WithDataType(binx.BinaryTypeVerbose(splits[0])))
		n, err := strconv.ParseUint(splits[1], 10, 64)
		if err != nil {
			return nil, err
		}
		options = append(options, WithLength(n))
	case 1:
		var dataType binx.BinaryTypeVerbose
		var length uint64 = 0
		switch splits[0] {
		case "int":
			dataType = binx.Int32
			length = 4
		case "uint":
			dataType = binx.Uint32
			length = 4
		case "int8":
			dataType = binx.Int8
			length = 1
		case "uint8":
			dataType = binx.Uint8
			length = 1
		case "int16":
			dataType = binx.Int16
			length = 2
		case "uint16":
			dataType = binx.Uint16
			length = 2
		case "int32":
			dataType = binx.Int32
			length = 4
		case "uint32":
			dataType = binx.Uint32
			length = 4
		case "int64":
			dataType = binx.Int64
			length = 8
		case "uint64":
			dataType = binx.Uint64
			length = 8
		default:
			dataType = "" // raw
			length = 1
		}
		options = append(options, WithDataType(dataType))
		options = append(options, WithLength(length))
	}
	return options, nil
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
func splitConfigAndNode(d any) ([]ConfigFunc, any) {
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
