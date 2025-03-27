package utils

import (
	"errors"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func ParseAppTagToOptions(template any, ext ...map[string]string) (configInfo []*ypb.ThirdPartyAppConfigItemTemplate, err error) {
	extTag := make(map[string]string)
	for _, m := range ext {
		for k, v := range m {
			extTag[k] = v
		}
	}
	defer func() {
		if r := recover(); r != nil {
			err = Error(r)
		}
	}()
	typeRef := reflect.TypeOf(template)
	varRef := reflect.ValueOf(template)
	if typeRef.Kind() == reflect.Ptr {
		typeRef = typeRef.Elem()
		varRef = varRef.Elem()
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
		parseKv := func(item *ypb.ThirdPartyAppConfigItemTemplate, tag string) error {
			splits := strings.Split(tag, ",")
			for _, split := range splits {
				if strings.Contains(split, ":") {
					kv := strings.Split(split, ":")
					if len(kv) == 2 {
						switch kv[0] {
						case "id":
							id, err := strconv.Atoi(kv[1])
							if err != nil {
								return Errorf("invalid id %s", kv[1])
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
							return Errorf("invalid tag %s", kv[0])
						}
					}
				}
			}
			return nil
		}
		if appTag != "" {
			item := &ypb.ThirdPartyAppConfigItemTemplate{}
			err = parseKv(item, appTag)
			if err != nil {
				return nil, err
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
			if !StringArrayContains([]string{"string", "number", "bool", "list"}, item.Type) {
				return nil, Errorf("invalid type %s", item.Type)
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

func ExportAppConfigToMap(ins any) (map[string]string, error) {
	res := map[string]string{}
	err := walkField(ins, func(field reflect.Value, tags map[string]string) {
		if v, ok := tags["name"]; ok {
			res[v] = InterfaceToString(field.Interface())
		}
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func ImportAppConfigToStruct(template any, data map[string]string) (err error) {
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
	for i := 0; i < typeRef.NumField(); i++ {
		field := typeRef.Field(i)
		tag := field.Tag
		appTag := tag.Get("app")
		if appTag != "" {
			splits := strings.Split(appTag, ",")
			tags := make(map[string]string)
			for _, split := range splits {
				if strings.Contains(split, ":") {
					kv := strings.Split(split, ":")
					if len(kv) != 2 {
						return Errorf("invalid tag %s", split)
					}
					tags[kv[0]] = kv[1]
				}
			}
			keyName, ok := tags["name"]
			if !ok {
				keyName = field.Name
			}
			v, ok := data[keyName]
			if !ok {
				v, ok = tags["default"]
				if !ok {
					continue
				}
			}
			if v == "" {
				continue
			}
			fieldValue := reflect.ValueOf(template).Elem().Field(i)
			switch field.Type.Kind() {
			case reflect.String:
				fieldValue.SetString(v)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intV, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return Errorf("invalid int value %s", v)
				}
				fieldValue.SetInt(intV)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				uintV, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return Errorf("invalid uint value %s", v)
				}
				fieldValue.SetUint(uintV)
			case reflect.Float32, reflect.Float64:
				floatV, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return Errorf("invalid float value %s", v)
				}
				fieldValue.SetFloat(floatV)
			case reflect.Bool:
				boolV, err := strconv.ParseBool(v)
				if err != nil {
					return Errorf("invalid bool value %s", v)
				}
				fieldValue.SetBool(boolV)
			default:
				return errors.New("unsupported field type")
			}
		}
	}
	return nil
}

func ParseAppTag(tag string) map[string]string {
	tagsMap := map[string]string{}
	splits := strings.Split(tag, ",")
	for _, split := range splits {
		if strings.Contains(split, ":") {
			kv := strings.Split(split, ":")
			if len(kv) == 2 {
				tagsMap[kv[0]] = kv[1]
			}
		}
	}
	return tagsMap
}

func walkField(template any, handle func(field reflect.Value, tags map[string]string)) error {
	typeRef := reflect.TypeOf(template)
	varRef := reflect.ValueOf(template)
	if typeRef.Kind() == reflect.Ptr {
		typeRef = typeRef.Elem()
		varRef = varRef.Elem()
	} else {
		return errors.New("template struct must be a pointer")
	}
	if typeRef.Kind() != reflect.Struct {
		return errors.New("template struct must be a struct")
	}
	for i := 0; i < typeRef.NumField(); i++ {
		field := typeRef.Field(i)
		tag := field.Tag
		appTag := tag.Get("app")
		handle(varRef.Field(i), ParseAppTag(appTag))
	}
	return nil
}
