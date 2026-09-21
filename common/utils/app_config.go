package utils

import (
	"errors"
	"github.com/yaklang/yaklang/common/utils/appconfig"
	"reflect"
	"strconv"
	"strings"
)

// ParseAppTagToOptions parses application tags into transport-independent descriptors.
// Service callers convert these descriptors to their wire format at the boundary.
func ParseAppTagToOptions(template any, ext ...map[string]string) ([]*appconfig.FieldDescriptor, error) {
	return appconfig.ParseAppTagToOptions(template, ext...)
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
