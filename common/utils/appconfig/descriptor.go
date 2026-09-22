// Package appconfig parses application configuration tags without service dependencies.
package appconfig

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// FieldDescriptor describes a tagged field independently of any transport schema.
type FieldDescriptor struct {
	Name         string
	Desc         string
	Required     bool
	Type         string
	DefaultValue string
	Verbose      string
	Extra        string
}

func ParseAppTagToOptions(template any, ext ...map[string]string) (configInfo []*FieldDescriptor, err error) {
	extTag := make(map[string]string)
	for _, m := range ext {
		for k, v := range m {
			extTag[k] = v
		}
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	typeRef := reflect.TypeOf(template)
	if typeRef.Kind() == reflect.Ptr {
		typeRef = typeRef.Elem()
	} else {
		return configInfo, errors.New("template struct must be a pointer")
	}
	if typeRef.Kind() != reflect.Struct {
		return configInfo, errors.New("template struct must be a struct")
	}
	idMap := make(map[*FieldDescriptor]int)
	for i := 0; i < typeRef.NumField(); i++ {
		field := typeRef.Field(i)
		tag := field.Tag
		appTag := tag.Get("app")
		parseKv := func(item *FieldDescriptor, tag string) error {
			splits := strings.Split(tag, ",")
			for _, split := range splits {
				if strings.Contains(split, ":") {
					kv := strings.Split(split, ":")
					if len(kv) == 2 {
						switch kv[0] {
						case "id":
							id, err := strconv.Atoi(kv[1])
							if err != nil {
								return fmt.Errorf("invalid id %s", kv[1])
							}
							idMap[item] = id
						case "name":
							item.Name = kv[1]
						case "desc":
							item.Desc = kv[1]
						case "required":
							item.Required = kv[1] == "true"
						case "type":
							item.Type = kv[1]
						case "default":
							item.DefaultValue = kv[1]
						case "verbose":
							item.Verbose = kv[1]
						case "extra":
							item.Extra = kv[1]
						default:
							return fmt.Errorf("invalid tag %s", kv[0])
						}
					}
				}
			}
			return nil
		}
		if appTag != "" {
			item := &FieldDescriptor{}
			err = parseKv(item, appTag)
			if err != nil {
				return nil, err
			}
			if item.Name == "" {
				item.Name = field.Name
			}
			if item.Verbose == "" {
				item.Verbose = item.Name
			}
			if item.Type == "" {
				typeName := ""
				switch field.Type.Kind().String() {
				case "string":
					typeName = "string"
				case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
					typeName = "number"
				case "bool":
					typeName = "bool"
				default:
					return nil, errors.New("unsupported field type")
				}
				item.Type = typeName
			}
			if item.Type != "string" && item.Type != "number" && item.Type != "bool" && item.Type != "list" {
				return nil, fmt.Errorf("invalid type %s", item.Type)
			}
			if extTags, ok := extTag[item.Name]; ok {
				err := parseKv(item, extTags)
				if err != nil {
					return nil, err
				}
			}
			configInfo = append(configInfo, item)
		}
	}
	sort.Slice(configInfo, func(i, j int) bool {
		return idMap[configInfo[i]] < idMap[configInfo[j]]
	})
	return configInfo, nil
}
