package yso

const (
	// CommonsCollections1/3/5/6/7链,需要<=3.2.1版本
	CC31Or321 = "org.apache.commons.collections.functors.ChainedTransformer"
	CC322     = "org.apache.commons.collections.ExtendedProperties$1"
	CC40      = "org.apache.commons.collections4.functors.ChainedTransformer"
	CC41      = "org.apache.commons.collections4.FluentIterable"
	// CommonsBeanutils2链,serialVersionUID不同,1.7x-1.8x为-3490850999041592962,1.9x为-2044202215314119608
	CB17  = "org.apache.commons.beanutils.MappedPropertyDescriptor$1"
	CB18x = "org.apache.commons.beanutils.DynaBeanMapDecorator$MapEntry"
	CB19x = "org.apache.commons.beanutils.BeanIntrospectionData"
	//c3p0 serialVersionUID不同,0.9.2pre2-0.9.5pre8为7387108436934414104,0.9.5pre9-0.9.5.5为7387108436934414104
	C3p092x = "com.mchange.v2.c3p0.impl.PoolBackedDataSourceBase"
	C3p095x = "com.mchange.v2.c3p0.test.AlwaysFailDataSource"
	// AspectJWeaver 需要cc31
	Ajw = "org.aspectj.weaver.tools.cache.SimpleCache"
	// bsh serialVersionUID不同,2.0b4为4949939576606791809,2.0b5为4041428789013517368,2.0.b6无法反序列化
	Bsh20b4 = "bsh.CollectionManager$1"
	Bsh20b5 = "bsh.engine.BshScriptEngine"
	Bsh20b6 = "bsh.collection.CollectionIterator$1"
	// Groovy 1.7.0-2.4.3,serialVersionUID不同,2.4.x为-8137949907733646644,2.3.x为1228988487386910280
	Groovy1702311 = "org.codehaus.groovy.reflection.ClassInfo$ClassInfoSet"
	Groovy24x     = "groovy.lang.Tuple2"
	Groovy244     = "org.codehaus.groovy.runtime.dgm$1170"
	// Becl JDK<8u251
	Becl    = "com.sun.org.apache.bcel.internal.util.ClassLoader"
	Jdk7u21 = "com.sun.corba.se.impl.orbutil.ORBClassLoader"
	// JRE8u20 7u25<=JDK<=8u20,虽然叫JRE8u20其实JDK8u20也可以,这个检测不完美,8u25版本以及JDK<=7u21会误报,可综合Jdk7u21来看
	JRE8u20   = "javax.swing.plaf.metal.MetalFileChooserUI$DirectoryComboBoxModel$1"
	LinuxOS   = "sun.awt.X11.AwtGraphicsConfigData"
	WindowsOS = "sun.awt.windows.WButtonPeer"
)

var allGadgetsCheckList = map[string]string{
	"CC31Or321":     CC31Or321,
	"CC322":         CC322,
	"CC40":          CC40,
	"CC41":          CC41,
	"CB17":          CB17,
	"CB18x":         CB18x,
	"CB19x":         CB19x,
	"C3p092x":       C3p092x,
	"C3p095x":       C3p095x,
	"Ajw":           Ajw,
	"Bsh20b4":       Bsh20b4,
	"Bsh20b5":       Bsh20b5,
	"Bsh20b6":       Bsh20b6,
	"Groovy1702311": Groovy1702311,
	"Groovy24x":     Groovy24x,
	"Groovy244":     Groovy244,
	"Becl":          Becl,
	"Jdk7u21":       Jdk7u21,
	"JRE8u20":       JRE8u20,
	"Linux_OS":      LinuxOS,
	"Windows_OS":    WindowsOS,
}

func GetGadgetChecklist() map[string]string {
	return allGadgetsCheckList
}
