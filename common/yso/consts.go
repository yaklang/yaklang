package yso

type ClassType string

const (
	ClassTcpReverseShell           ClassType = "TcpReverseShell"
	ClassSleep                     ClassType = "Sleep"
	ClassTomcatEcho                ClassType = "TomcatEcho"
	ClassMultiEcho                 ClassType = "MultiEcho"
	ClassModifyTomcatMaxHeaderSize ClassType = "ModifyTomcatMaxHeaderSize"
	ClassRuntimeExec               ClassType = "RuntimeExec"
	ClassProcessBuilderExec        ClassType = "ProcessBuilderExec"
	ClassTcpReverse                ClassType = "TcpReverse"
	ClassProcessImplExec           ClassType = "ProcessImplExec"
	ClassDNSLog                    ClassType = "DNSLog"
	ClassSpringEcho                ClassType = "SpringEcho"
	ClassTemplateImplClassLoader   ClassType = "TemplateImplClassLoader"
	ClassEmptyClassInTemplate      ClassType = "EmptyClassInTemplate"
)

type ClassParamType string

const (
	ClassParamMax         ClassParamType = "max"
	ClassParamHost        ClassParamType = "host"
	ClassParamHeader      ClassParamType = "header"
	ClassParamAction      ClassParamType = "action"
	ClassParamHeaderAuKey ClassParamType = "header-au-key"
	ClassParamPort        ClassParamType = "port"
	ClassParamHeaderAuVal ClassParamType = "header-au-val"
	ClassParamDomain      ClassParamType = "domain"
	ClassParamCmd         ClassParamType = "cmd"
	ClassParamToken       ClassParamType = "token"
	ClassParamBase64Class ClassParamType = "base64Class"
	ClassParamTime        ClassParamType = "time"
	ClassParamPosition    ClassParamType = "position"
)

type GadgetType string

const (
	GadgetCommonsCollections4     GadgetType = "CommonsCollections4"
	GadgetCommonsCollections8     GadgetType = "CommonsCollections8"
	GadgetSpring1                 GadgetType = "Spring1"
	GadgetCommonsCollections5     GadgetType = "CommonsCollections5"
	GadgetJdk8u20                 GadgetType = "Jdk8u20"
	GadgetHibernate1              GadgetType = "Hibernate1"
	GadgetCommonsBeanutils1       GadgetType = "CommonsBeanutils1"
	GadgetJBossInterceptors1      GadgetType = "JBossInterceptors1"
	GadgetCommonsCollections6     GadgetType = "CommonsCollections6"
	GadgetCommonsCollectionsK2    GadgetType = "CommonsCollectionsK2"
	GadgetJavassistWeld1          GadgetType = "JavassistWeld1"
	GadgetCommonsCollections10    GadgetType = "CommonsCollections10"
	GadgetCommonsCollections11    GadgetType = "CommonsCollections11"
	GadgetCommonsBeanutils1_183   GadgetType = "CommonsBeanutils1_183"
	GadgetCommonsBeanutils2       GadgetType = "CommonsBeanutils2"
	GadgetCommonsCollectionsK4    GadgetType = "CommonsCollectionsK4"
	GadgetBeanShell1              GadgetType = "BeanShell1"
	GadgetClick1                  GadgetType = "Click1"
	GadgetJSON1                   GadgetType = "JSON1"
	GadgetROME                    GadgetType = "ROME"
	GadgetCommonsBeanutils2_183   GadgetType = "CommonsBeanutils2_183"
	GadgetSpring2                 GadgetType = "Spring2"
	GadgetCommonsCollections6Lite GadgetType = "CommonsCollections6Lite"
	GadgetMozillaRhino1           GadgetType = "MozillaRhino1"
	GadgetCommonsCollections1     GadgetType = "CommonsCollections1"
	GadgetCommonsCollectionsK3    GadgetType = "CommonsCollectionsK3"
	GadgetCommonsCollections3     GadgetType = "CommonsCollections3"
	GadgetCommonsCollectionsK1    GadgetType = "CommonsCollectionsK1"
	GadgetMozillaRhino2           GadgetType = "MozillaRhino2"
	GadgetVaadin1                 GadgetType = "Vaadin1"
	GadgetCommonsCollections7     GadgetType = "CommonsCollections7"
	GadgetCommonsCollections9     GadgetType = "CommonsCollections9"
	GadgetGroovy1                 GadgetType = "Groovy1"
	GadgetJdk7u21                 GadgetType = "Jdk7u21"
	GadgetCommonsCollections2     GadgetType = "CommonsCollections2"
)
