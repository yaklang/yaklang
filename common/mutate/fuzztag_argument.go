package mutate

import (
	"errors"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/fuzztagx"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
)

type FuzztagArgumentType struct {
	Name        string
	Select      []*FuzztagArgumentType
	Default     any
	Description string
	Separator   []string
	IsOptional  bool
	IsList      bool
}

var typeMap = make(map[string]byte)
var typeMapReverse = make(map[byte]string)

func init() {
	typeDefinedStr := "String:S,Number:N,Enum:E"
	for _, s := range strings.Split(typeDefinedStr, ",") {
		name, flag, ok := strings.Cut(s, ":")
		if !ok {
			continue
		}
		if _, ok := typeMap[name]; ok {
			log.Warnf("type %s has been defined", name)
			continue
		}
		if _, ok := typeMapReverse[flag[0]]; ok {
			log.Warnf("type flag %c has been defined", flag[0])
			continue
		}
		typeMap[name] = flag[0]
		typeMapReverse[flag[0]] = name
	}
}

var InvalidTypeFormat = errors.New("invalid type format")

func ParseFuzztagArgumentTypes(description string) ([]*FuzztagArgumentType, error) {
	idToType := make(map[byte]*FuzztagArgumentType)
	var id byte = 10
	newTypeRef := func(typ *FuzztagArgumentType) byte {
		id++
		idToType[id] = typ
		return id
	}
	parseType := func(name string, separator []string, s string) ([]*parser.FuzzResult, error) {
		pre, after, ok := strings.Cut(s, ":")
		if !ok {
			return nil, InvalidTypeFormat
		}
		var defaultVal any = pre
		switch name {
		case "number":
			n, err := strconv.Atoi(pre)
			if err != nil {
				return nil, err
			}
			defaultVal = n
		case "range":
			pre, after, ok := strings.Cut(pre, "-")
			if !ok {
				return nil, InvalidTypeFormat
			}
			start, err := strconv.Atoi(pre)
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(after)
			if err != nil {
				return nil, err
			}
			defaultVal = [2]int{start, end}
		}
		typIns := &FuzztagArgumentType{
			Name:        name,
			Default:     defaultVal,
			Separator:   separator,
			Description: after,
		}
		id := newTypeRef(typIns)
		res := parser.NewFuzzResultWithData([]byte{id})
		return []*parser.FuzzResult{res}, nil
	}
	parseEnum := func(s string, separator []string) ([]*parser.FuzzResult, error) {
		refIds, after, ok := strings.Cut(s, ":")
		if !ok {
			return nil, InvalidTypeFormat
		}
		typIns := &FuzztagArgumentType{}
		for _, refId := range []byte(refIds) {
			typIns.Select = append(typIns.Select, idToType[refId])
		}
		defaultVal, after, ok := strings.Cut(after, ":")
		if !ok {
			return nil, InvalidTypeFormat
		}
		typIns.Name = "Enum"
		typIns.Default = defaultVal
		typIns.Description = after
		typIns.Separator = separator
		id := newTypeRef(typIns)
		res := parser.NewFuzzResultWithData([]byte{id})
		return []*parser.FuzzResult{res}, nil
	}
	optional := func(s string) ([]*parser.FuzzResult, error) {
		if len(s) != 1 {
			return nil, InvalidTypeFormat
		}
		idToType[s[0]].IsOptional = true
		res := parser.NewFuzzResultWithData([]byte{id})
		return []*parser.FuzzResult{res}, nil
	}
	list := func(s string) ([]*parser.FuzzResult, error) {
		if len(s) != 1 {
			return nil, InvalidTypeFormat
		}
		idToType[s[0]].IsList = true
		res := parser.NewFuzzResultWithData([]byte{id})
		return []*parser.FuzzResult{res}, nil
	}
	methods := map[string]*parser.TagMethod{
		"optional": {
			Fun: optional,
		},
		"list": {
			Fun: list,
		},
	}
	typeNames := []string{"string", "number", "range"}

	separatorMap := map[string][]string{"split": {"|"}, "contact": {"-"}, "dot": {","}, "split_dot": {"|", ","}}
	for sepName, sep := range separatorMap {
		methods["enum"+"_"+sepName] = &parser.TagMethod{
			Fun: func(s string) ([]*parser.FuzzResult, error) {
				return parseEnum(s, sep)
			},
		}
	}
	methods["enum"] = &parser.TagMethod{
		Fun: func(s string) ([]*parser.FuzzResult, error) {
			return parseEnum(s, []string{","})
		},
	}
	for _, name := range typeNames {
		name := name
		for sepName, sep := range separatorMap {
			sep := sep
			methods[name+"_"+sepName] = &parser.TagMethod{
				Fun: func(s string) ([]*parser.FuzzResult, error) {
					return parseType(name, sep, s)
				},
			}
		}
		methods[name] = &parser.TagMethod{
			Fun: func(s string) ([]*parser.FuzzResult, error) {
				return parseType(name, []string{","}, s)
			},
		}
	}
	generator, err := fuzztagx.NewGenerator(description, methods, true, false)
	if err != nil {
		return nil, err
	}
	ok := generator.Next()
	if !ok {
		return nil, generator.Error
	}
	res := generator.Result().GetData()
	typeList := make([]*FuzztagArgumentType, 0)
	for _, id := range res {
		typ := idToType[id]
		typeList = append(typeList, typ)
	}
	return typeList, nil
}

func DumpFuzztagArgumentTypes(typ []*FuzztagArgumentType) string {
	res := []string{}
	for _, argumentType := range typ {
		if argumentType.Name == "Enum" {
			s := fmt.Sprintf("Name: %s, Default: %v, Description: %s Select: %s IsOptional: %v IsList: %v", argumentType.Name, argumentType.Default, argumentType.Description, DumpFuzztagArgumentTypes(argumentType.Select), argumentType.IsOptional, argumentType.IsList)
			res = append(res, s)
			continue
		}
		s := fmt.Sprintf("Name: %s, Default: %v, Description: %s Separator: %s IsOptional: %v IsList: %v", argumentType.Name, argumentType.Default, argumentType.Description, argumentType.Separator, argumentType.IsOptional, argumentType.IsList)
		res = append(res, s)
	}
	res = lo.Map(res, func(item string, index int) string {
		return fmt.Sprintf("{{%s}}", item)
	})
	return strings.Join(res, ",")
}

func GenerateExampleTags(tag *FuzzTagDescription) ([]string, error) {
	args := tag.ArgumentDescription
	if args == "" {
		return []string{fmt.Sprintf("{{%s}}", tag.TagName)}, nil
	}
	types, err := ParseFuzztagArgumentTypes(args)
	if err != nil {
		return nil, err
	}
	var generateParams func(types []*FuzztagArgumentType) []string
	generateParams = func(types []*FuzztagArgumentType) []string {
		res := []string{}
		if len(types) == 0 {
			return []string{""}
		}
		getDefaultVal := func(typ *FuzztagArgumentType) string {
			if typ.Name == "range" {
				r := typ.Default.([2]int)
				return fmt.Sprintf("%d-%d", r[0], r[1])
			}
			return utils.InterfaceToString(typ.Default)
		}
		if types[0].IsList {
			for _, sep := range types[0].Separator {
				p := getDefaultVal(types[0]) + sep
				res = append(res, "")
				res = append(res, p+p)
			}
		} else if types[0].IsOptional {
			for _, sep := range types[0].Separator {
				p := getDefaultVal(types[0]) + sep
				res = append(res, "")
				res = append(res, p)
			}
		} else {
			for _, sep := range types[0].Separator {
				p := getDefaultVal(types[0]) + sep
				res = append(res, p)
			}
		}
		tags := generateParams(types[1:])
		newRes := []string{}
		for _, tag := range tags {
			for _, re := range res {
				newRes = append(newRes, re+tag)
			}
		}
		return newRes
	}
	params := generateParams(types)
	res := lo.Map(params, func(param string, index int) string {
		for _, s := range []string{",", "-", "|"} {
			if strings.HasSuffix(param, s) {
				param = param[:len(param)-len(s)]
			}
		}
		return fmt.Sprintf("{{%s(%s)}}", tag.TagName, param)
	})
	return res, nil
}
