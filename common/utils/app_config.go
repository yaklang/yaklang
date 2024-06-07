package utils

import (
	"errors"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func LoadAppConfig(template any) (configInfo []*ypb.ThirdPartyAppConfigItemTemplate, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = Error(r)
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
	idMap := make(map[*ypb.ThirdPartyAppConfigItemTemplate]int)
	for i := 0; i < typeRef.NumField(); i++ {
		field := typeRef.Field(i)
		tag := field.Tag
		appTag := tag.Get("app")
		if appTag != "" {
			item := &ypb.ThirdPartyAppConfigItemTemplate{}
			splits := strings.Split(appTag, ",")
			for _, split := range splits {
				if strings.Contains(split, ":") {
					kv := strings.Split(split, ":")
					if len(kv) == 2 {
						switch kv[0] {
						case "id":
							id, err := strconv.Atoi(kv[1])
							if err != nil {
								return nil, Errorf("invalid id %s", kv[1])
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
						case "verbose":
							item.Verbose = kv[1]
						case "default":
							item.DefaultValue = kv[1]
						case "extra":
							item.Extra = kv[1]
						default:
							return nil, Errorf("invalid tag %s", kv[0])
						}
					}
				}
			}
			//if item.Name == "" {
			//	item.Name = field.Name
			//}
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
			if !StringArrayContains([]string{"string", "number", "bool"}, item.Type) {
				return nil, Errorf("invalid type %s", item.Type)
			}
			configInfo = append(configInfo, item)
		}
	}
	sort.Slice(configInfo, func(i, j int) bool {
		return idMap[configInfo[i]] < idMap[configInfo[j]]
	})
	return configInfo, nil
}

func ApplyAppConfig(template any, opts []*ypb.ThirdPartyAppConfigItemTemplate) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = Error(r)
		}
	}()
	typeRef := reflect.TypeOf(template)
	if typeRef.Kind() == reflect.Ptr {
		typeRef = typeRef.Elem()
	} else {
		return errors.New("template struct must be a pointer")
	}
	if typeRef.Kind() != reflect.Struct {
		return errors.New("template struct must be a struct")
	}
	for _, opt := range opts {
		for i := 0; i < typeRef.NumField(); i++ {
			field := typeRef.Field(i)
			tag := field.Tag
			appTag := tag.Get("app")
			if appTag != "" {
				splits := strings.Split(appTag, ",")
				for _, split := range splits {
					if strings.Contains(split, ":") {
						kv := strings.Split(split, ":")
						if len(kv) == 2 {
							if kv[0] == "name" && kv[1] == opt.Name {
								value := reflect.ValueOf(template).Elem().Field(i)
								if opt.DefaultValue != "" {
									switch value.Kind() {
									case reflect.String:
										value.SetString(opt.DefaultValue)
									case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
										v, err := strconv.ParseInt(opt.DefaultValue, 10, 64)
										if err != nil {
											return err
										}
										value.SetInt(v)
									case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
										v, err := strconv.ParseUint(opt.DefaultValue, 10, 64)
										if err != nil {
											return err
										}
										value.SetUint(v)
									case reflect.Float32, reflect.Float64:
										v, err := strconv.ParseFloat(opt.DefaultValue, 64)
										if err != nil {
											return err
										}
										value.SetFloat(v)
									case reflect.Bool:
										value.SetBool(opt.DefaultValue == "true")
									default:
										return errors.New("unsupported field type")
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}
