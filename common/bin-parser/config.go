package bin_parser

import (
	"errors"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v2"
	"strconv"
	"strings"
)

type Config struct {
	endian   string
	dataType string
	length   int
}

func NewConfig(opts []ConfigFunc) *Config {
	cfg := &Config{
		endian:   "big",
		dataType: "raw",
		length:   0,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

type ConfigFunc func(config *Config)

func WithEndian(s string) ConfigFunc {
	return func(config *Config) {
		config.endian = s
	}
}
func WithDataType(s string) ConfigFunc {
	return func(config *Config) {
		config.dataType = s
	}
}
func WithLength(l int) ConfigFunc {
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
			return nil, errors.New("invalid type")
		}
		options = append(options, WithDataType(splits[0]))
		n, err := strconv.Atoi(splits[1])
		if err != nil {
			return nil, err
		}
		options = append(options, WithLength(n))
	case 1:
		dataType := ""
		length := 0
		switch splits[0] {
		case "int":
			dataType = "int"
			length = 4
		case "uint":
			dataType = "uint"
			length = 4
		case "int8":
			dataType = "int"
			length = 1
		case "uint8":
			dataType = "uint"
			length = 1
		case "int16":
			dataType = "int"
			length = 2
		case "uint16":
			dataType = "uint"
			length = 2
		case "int32":
			dataType = "int"
			length = 4
		case "uint32":
			dataType = "uint"
			length = 4
		case "int64":
			dataType = "int"
			length = 8
		case "uint64":
			dataType = "uint"
			length = 8
		case "float32":
			dataType = "float"
			length = 4
		case "float64":
			dataType = "float"
			length = 8
		case "string":
			dataType = "string"
		default:
			return nil, errors.New("invalid terminal node")
		}
		options = append(options, WithDataType(dataType))
		options = append(options, WithLength(length))
	}
	return options, nil
}

// getNode 从node中提取配置信息
func splitConfigAndNode(d any) ([]ConfigFunc, any) {
	defaultConfig := []ConfigFunc{WithEndian("big")}
	var getConfigFromSlice func(d any) ([]ConfigFunc, any)
	getConfigFromSlice = func(d any) ([]ConfigFunc, any) {
		switch ret := d.(type) {
		case yaml.MapSlice:
			if len(ret) > 0 {
				if ret[0].Key == "config" {
					config, _ := getConfigFromSlice(ret[0].Value)
					return config, ret[1:]
				}
				if ret[0].Key == "endian" {
					if strings.ToLower(utils.InterfaceToString(ret[0].Value)) != "big" {
						WithEndian("little")
						return defaultConfig, ret[1:]
					} else {
						return defaultConfig, ret[1:]
					}
				}
			} else {
				return defaultConfig, ret
			}
		}
		return defaultConfig, d
	}
	cfg, node := getConfigFromSlice(d)
	if v, ok := node.(yaml.MapSlice); ok && len(v) == 1 && v[0].Key == "Data" {
		return cfg, v[0]
	}
	return cfg, node
}
