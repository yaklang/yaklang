package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestOop(t *testing.T) {
	code := `package com.example.demo1;

import java.util.Map;

public class Kls1 {
    public static void DoGet(String url, Map<String, String> map) {
        BeanFactory.create.defaultHandler(url, map);
    }

    public static void doGet() {
        return DoGet("/api/fast.json", null);
    }
}

`
	ssatest.CheckSyntaxFlow(t, code, `.create.defaultHandler(* #-> * as $param)`, map[string][]string{
		"param": {`"/api/fast.json"`, "nil", "Undefined-BeanFactory"},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
func TestOopInterface(t *testing.T) {
	code := `package com.example.demo1;


class IFunc {
    public int DoGet(String url);
}

class ImplB implements IFunc {
    @Override
    public int DoGet(String url) {
        return 2;
    }
}

public class Main {
    private IFunc Ifunc;
	private ImplB implb;

    public void main(String[] args) {
        func0(Ifunc.DoGet("123"));
    }
}`
	ssatest.CheckSyntaxFlow(t, code, `func0(* #-> * as $param)`, map[string][]string{
		"param": {"Function-IFunc.DoGet", "2"},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestTypename(t *testing.T) {
	code := `
package com.example.springboot.controller;
import com.thoughtworks.xstream.XStream;
class test{
	public XStream xstreamInstance = null;
    public void createPerson() {
        xstreamInstance.alias("person", Person.class);
    }
	public void XX(){
	}
}
}
`
	t.Run("test typename field", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `.alias<getObject><typeName> as $output`,
			map[string][]string{"output": {`"com.thoughtworks.xstream.XStream"`, `"XStream"`}},
			ssaapi.WithLanguage(ssaconfig.JAVA))
	})
	t.Run("test typename member method", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `.alias<typeName> as $output`,
			map[string][]string{"output": {`"com.thoughtworks.xstream.XStream"`, `"XStream"`}},
			ssaapi.WithLanguage(ssaconfig.JAVA))
	},
	)
}
func TestConstructor(t *testing.T) {
	t.Run("have constructor", func(t *testing.T) {
		code := `package main;
class test{
	public test(){
	}
	public void A(){
		test t = new test();
	}
}
`
		ssatest.CheckSyntaxFlow(t, code, `test() as $output`, map[string][]string{
			"output": {"Function-test.test(Undefined-test)"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
	t.Run("no constructor", func(t *testing.T) {
		code := `package main;
class test{
	public void A(){
		test t = new test();
	}
}
`
		ssatest.CheckSyntaxFlow(t, code, `test() as $output`, map[string][]string{
			"output": {"Undefined-test(Undefined-test)"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
