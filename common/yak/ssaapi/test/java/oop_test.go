package java

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
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
	}, ssaapi.WithLanguage(ssaapi.JAVA))
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
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
