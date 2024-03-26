package yso

var Exports = map[string]interface{}{
	// 生成链
	"ToBytes": ToBytes,
	"ToBcel":  ToBcel,
	"ToJson":  ToJson,
	"dump":    Dump,
	//JavaObject
	"GetJavaObjectFromBytes":  GetJavaObjectFromBytes,
	"GetBeanShell1JavaObject": GetBeanShell1JavaObject,
	"GetClick1JavaObject":     GetClick1JavaObject,
	//"GetClojureJavaObject":                 GetClojureJavaObject,
	"GetCommonsBeanutils1JavaObject":       GetCommonsBeanutils1JavaObject,
	"GetCommonsBeanutils183NOCCJavaObject": GetCommonsBeanutils183NOCCJavaObject,
	"GetCommonsBeanutils192NOCCJavaObject": GetCommonsBeanutils192NOCCJavaObject,
	"GetCommonsCollections1JavaObject":     GetCommonsCollections1JavaObject,
	"GetCommonsCollections2JavaObject":     GetCommonsCollections2JavaObject,
	"GetCommonsCollections3JavaObject":     GetCommonsCollections3JavaObject,
	"GetCommonsCollections4JavaObject":     GetCommonsCollections4JavaObject,
	"GetCommonsCollections5JavaObject":     GetCommonsCollections5JavaObject,
	"GetCommonsCollections6JavaObject":     GetCommonsCollections6JavaObject,
	"GetCommonsCollections7JavaObject":     GetCommonsCollections7JavaObject,
	"GetCommonsCollections8JavaObject":     GetCommonsCollections8JavaObject,
	"GetCommonsCollectionsK1JavaObject":    GetCommonsCollectionsK1JavaObject,
	"GetCommonsCollectionsK2JavaObject":    GetCommonsCollectionsK2JavaObject,
	"GetCommonsCollectionsK3JavaObject":    GetCommonsCollectionsK3JavaObject,
	"GetCommonsCollectionsK4JavaObject":    GetCommonsCollectionsK4JavaObject,
	"GetGroovy1JavaObject":                 GetGroovy1JavaObject,
	"GetJBossInterceptors1JavaObject":      GetJBossInterceptors1JavaObject,
	"GetURLDNSJavaObject":                  GetURLDNSJavaObject,
	"GetFindGadgetByDNSJavaObject":         GetFindGadgetByDNSJavaObject,

	// 通过gadget名称获取gadget
	"GetGadget":       GenerateGadget,
	"WarpByDirtyData": WarpSerializeDataByDirtyData,

	//"GetJRMPClientJavaObject":              GetJRMPClientJavaObject,
	"GetJSON1JavaObject":          GetJSON1JavaObject,
	"GetJavassistWeld1JavaObject": GetJavassistWeld1JavaObject,
	"GetJdk7u21JavaObject":        GetJdk7u21JavaObject,
	"GetJdk8u20JavaObject":        GetJdk8u20JavaObject,
	//批量获取Gadget
	"GetAllGadget":            GetAllGadget,
	"GetAllTemplatesGadget":   GetAllTemplatesGadget,
	"GetAllRuntimeExecGadget": GetAllRuntimeExecGadget,
	//获取Gadget名称
	"GetGadgetNameByFun": GetGadgetNameByFun,
	//用于Shiro检查
	"GetSimplePrincipalCollectionJavaObject": GetSimplePrincipalCollectionJavaObject,
	// 加载 java class
	"LoadClassFromBytes":  LoadClassFromBytes,
	"LoadClassFromBase64": LoadClassFromBase64,
	"LoadClassFromBCEL":   LoadClassFromBCEL,

	// 只生成恶意类的对象
	"GenerateClass":                                    GenerateClass,
	"useClassParam":                                    SetClassParam,
	"useTemplate":                                      SetClassType,
	"GenerateClassObjectFromBytes":                     GenerateClassObjectFromBytes,
	"GenerateRuntimeExecEvilClassObject":               GenerateRuntimeExecEvilClassObject,
	"GenerateProcessBuilderExecEvilClassObject":        GenerateProcessBuilderExecEvilClassObject,
	"GenerateProcessImplExecEvilClassObject":           GenerateProcessImplExecEvilClassObject,
	"GenerateDNSlogEvilClassObject":                    GenDnslogClassObject,
	"GenerateSpringEchoEvilClassObject":                GenerateSpringEchoEvilClassObject,
	"GenerateModifyTomcatMaxHeaderSizeEvilClassObject": GenerateModifyTomcatMaxHeaderSizeEvilClassObject,
	"GenerateTcpReverseEvilClassObject":                GenTcpReverseClassObject,
	"GenerateTcpReverseShellEvilClassObject":           GenTcpReverseShellClassObject,
	"GenerateTomcatEchoClassObject":                    GenTomcatEchoClassObject,
	"GenerateMultiEchoClassObject":                     GenMultiEchoClassObject,
	"GenerateHeaderEchoClassObject":                    GenHeaderEchoClassObject,
	"GenerateSleepClassObject":                         GenSleepClassObject,
	// bytes class
	"useBytesEvilClass":         SetBytesEvilClass,
	"useBytesClass":             SetClassBytes,
	"useBase64BytesClass":       SetClassBase64Bytes,
	"useTomcatEchoEvilClass":    SetTomcatEchoEvilClass,
	"useTomcatEchoTemplate":     SetClassTomcatEchoTemplate,
	"useMultiEchoEvilClass":     SetMultiEchoEvilClass,
	"useClassMultiEchoTemplate": SetClassMultiEchoTemplate,
	//ModifyTomcatMaxHeaderSize
	"useModifyTomcatMaxHeaderSizeTemplate": SetClassModifyTomcatMaxHeaderSizeTemplate,
	//springecho template
	"useSpringEchoTemplate":   SetClassSpringEchoTemplate,
	"springHeader":            SetHeader,
	"springParam":             SetParam,
	"springRuntimeExecAction": SetExecAction,
	"springEchoBody":          SetEchoBody,
	// Dnslog template
	"useDNSlogTemplate":  SetClassDnslogTemplate,
	"dnslogDomain":       SetDnslog,
	"useDNSLogEvilClass": SetDnslogEvilClass,
	// runtime exec template
	"useRuntimeExecTemplate":  SetClassRuntimeExecTemplate,
	"command":                 SetExecCommand,
	"majorVersion":            SetMajorVersion,
	"useRuntimeExecEvilClass": SetRuntimeExecEvilClass,
	// runtime exec template
	"useProcessBuilderExecTemplate":  SetClassProcessBuilderExecTemplate,
	"useProcessBuilderExecEvilClass": SetProcessBuilderExecEvilClass,
	// runtime exec template
	"useProcessImplExecTemplate":  SetClassProcessImplExecTemplate,
	"useProcessImplExecEvilClass": SetProcessImplExecEvilClass,
	// tcp reverse template
	"useTcpReverseTemplate":  SetClassTcpReverseTemplate,
	"tcpReverseHost":         SetTcpReverseHost,
	"tcpReversePort":         SetTcpReversePort,
	"tcpReverseToken":        SetTcpReverseToken,
	"useTcpReverseEvilClass": SetTcpReverseEvilClass,
	// tcp reverse shell template
	"useTcpReverseShellTemplate":  SetClassTcpReverseShellTemplate,
	"useTcpReverseShellEvilClass": SetTcpReverseShellEvilClass,
	// header echo template
	"useHeaderEchoTemplate":  SetClassHeaderEchoTemplate,
	"useHeaderEchoEvilClass": SetHeaderEchoEvilClass,
	"useEchoBody":            SetEchoBody,
	"useParam":               SetParam,
	"useHeaderParam":         SetHeader,
	// sleep template
	"useSleepTemplate":  SetClassSleepTemplate,
	"useSleepEvilClass": SetSleepEvilClass,
	"useSleepTime":      SetSleepTime,
	// 其他设置
	"useConstructorExecutor":       SetConstruct, // 使用构造器执行
	"evilClassName":                SetClassName, // className
	"obfuscationClassConstantPool": SetObfuscation,
}
