package yso

type ClassType string

const (
	ClassSleep                     ClassType = "Sleep"
	ClassSpringEcho                ClassType = "SpringEcho"
	ClassDNSLog                    ClassType = "DNSLog"
	ClassRuntimeExec               ClassType = "RuntimeExec"
	ClassProcessImplExec           ClassType = "ProcessImplExec"
	ClassTomcatEcho                ClassType = "TomcatEcho"
	ClassTcpReverseShell           ClassType = "TcpReverseShell"
	ClassEmptyClassInTemplate      ClassType = "EmptyClassInTemplate"
	ClassProcessBuilderExec        ClassType = "ProcessBuilderExec"
	ClassTcpReverse                ClassType = "TcpReverse"
	ClassTemplateImplClassLoader   ClassType = "TemplateImplClassLoader"
	ClassModifyTomcatMaxHeaderSize ClassType = "ModifyTomcatMaxHeaderSize"
	ClassMultiEcho                 ClassType = "MultiEcho"
)

type ClassParamType string

const (
	ClassParamPort        ClassParamType = "port"
	ClassParamHeaderAuKey ClassParamType = "header-au-key"
	ClassParamTime        ClassParamType = "time"
	ClassParamHost        ClassParamType = "host"
	ClassParamHeaderAuVal ClassParamType = "header-au-val"
	ClassParamMax         ClassParamType = "max"
	ClassParamPosition    ClassParamType = "position"
	ClassParamToken       ClassParamType = "token"
	ClassParamBase64Class ClassParamType = "base64Class"
	ClassParamDomain      ClassParamType = "domain"
	ClassParamCmd         ClassParamType = "cmd"
	ClassParamHeader      ClassParamType = "header"
	ClassParamAction      ClassParamType = "action"
)

type GadgetType string

const (
	GadgetSpring2                   GadgetType = "Spring2"
	GadgetJdk8u20                   GadgetType = "Jdk8u20"
	GadgetCommonsCollections11      GadgetType = "CommonsCollections11"
	GadgetURLDNS                    GadgetType = "URLDNS"
	GadgetCommonsCollections3       GadgetType = "CommonsCollections3"
	GadgetCommonsCollections2       GadgetType = "CommonsCollections2"
	GadgetJavassistWeld1            GadgetType = "JavassistWeld1"
	GadgetBeanShell1                GadgetType = "BeanShell1"
	GadgetCommonsCollections6       GadgetType = "CommonsCollections6"
	GadgetMozillaRhino1             GadgetType = "MozillaRhino1"
	GadgetSimplePrincipalCollection GadgetType = "SimplePrincipalCollection"
	GadgetCommonsCollections6Lite   GadgetType = "CommonsCollections6Lite"
	GadgetCommonsCollections5       GadgetType = "CommonsCollections5"
	GadgetCommonsCollectionsK1      GadgetType = "CommonsCollectionsK1"
	GadgetSpring1                   GadgetType = "Spring1"
	GadgetCommonsCollections9       GadgetType = "CommonsCollections9"
	GadgetFindAllClassesByDNS       GadgetType = "FindAllClassesByDNS"
	GadgetCommonsBeanutils2         GadgetType = "CommonsBeanutils2"
	GadgetHibernate1                GadgetType = "Hibernate1"
	GadgetGroovy1                   GadgetType = "Groovy1"
	GadgetCommonsCollectionsK4      GadgetType = "CommonsCollectionsK4"
	GadgetJSON1                     GadgetType = "JSON1"
	GadgetJdk7u21                   GadgetType = "Jdk7u21"
	GadgetCommonsCollectionsK2      GadgetType = "CommonsCollectionsK2"
	GadgetCommonsCollections1       GadgetType = "CommonsCollections1"
	GadgetVaadin1                   GadgetType = "Vaadin1"
	GadgetCommonsCollections4       GadgetType = "CommonsCollections4"
	GadgetROME                      GadgetType = "ROME"
	GadgetCommonsCollectionsK3      GadgetType = "CommonsCollectionsK3"
	GadgetCommonsCollections10      GadgetType = "CommonsCollections10"
	GadgetJBossInterceptors1        GadgetType = "JBossInterceptors1"
	GadgetCommonsBeanutils1_183     GadgetType = "CommonsBeanutils1_183"
	GadgetClick1                    GadgetType = "Click1"
	GadgetCommonsBeanutils1         GadgetType = "CommonsBeanutils1"
	GadgetMozillaRhino2             GadgetType = "MozillaRhino2"
	GadgetCommonsCollections8       GadgetType = "CommonsCollections8"
	GadgetCommonsBeanutils2_183     GadgetType = "CommonsBeanutils2_183"
	GadgetFindClassByBomb           GadgetType = "FindClassByBomb"
	GadgetCommonsCollections7       GadgetType = "CommonsCollections7"
	GadgetFindClassByDNS            GadgetType = "FindClassByDNS"
	GadgetCommonsBeanutils3         GadgetType = "CommonsBeanutils3"
)
