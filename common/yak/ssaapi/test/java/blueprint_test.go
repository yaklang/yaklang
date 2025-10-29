package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBlueprint(t *testing.T) {
	code := `package main;
class B {
    public int b = 1;
}

class A {
    public B a;

    public void A() {
        System.out.println(this.a.b);
    }
}

public class Main {
    public static void main(String[] args) {
        A a = new A();
        a.a = new B();
        a.A();  // 显式调用方法
    }
}
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{"param": {"1", "Undefined-System"}}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestBlueprintFullTypeName(t *testing.T) {
	t.Run("test blueprint virtual import", func(t *testing.T) {
		code := `package com.example.servlet;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.servlet.http.HttpSession;

public class DemoServlet extends HttpServlet {
	protected void doGet(HttpServletRequest request, HttpServletResponse response) {}
}
`
		ssatest.CheckSyntaxFlow(t, code, `request<fullTypeName> as $output`, map[string][]string{
			"output": {`"javax.servlet.http.HttpServletRequest"`},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
	t.Run("test blueprint virtual import,fullTypeName is more", func(t *testing.T) {
		code := `package com.example.servlet;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.*;
import javax.servlet.http.HttpSession;

public class DemoServlet extends HttpServlet {
	protected void doGet(HttpServletRequest request, HttpServletResponse response) {}
}`
		ssatest.CheckSyntaxFlow(t, code, `request<fullTypeName> as $output`, map[string][]string{
			"output": {
				`"javax.servlet.http.HttpServletRequest"`,
				`"com.example.servlet.HttpServletRequest"`,
				`"java.lang.HttpServletRequest"`,
			},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
	t.Run("blueprint keyword", func(t *testing.T) {
		code := `package com.example.servlet;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
class A {
	public static void main(String[] args) {
		try(InputStream  in = new FileInputStream(src)){}
	}
}
`
		ssatest.CheckSyntaxFlow(t, code, `in<fullTypeName> as $output`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
func TestNativeCallBlueprint(t *testing.T) {
	code := `package main;
public class OuterClass {
    public static void main(String[] args) {
    }
}
`
	ssatest.CheckSyntaxFlow(t, code, `main<getCurrentBlueprint><fullTypeName> as $sink`, map[string][]string{
		"sink": {`"main.OuterClass"`},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
func TestBlueprintBottomUse(t *testing.T) {
	t.Run("test new creator", func(t *testing.T) {
		code := `package main;
class A{
	public void main(){
		String ip = request.getParameter("ip");
		String[] cmd = new String[]{"ping", "-c", "2", ip};
		Runtime rt = Runtime.getRuntime();
        Process proc = rt.exec(cmd);
	}
}`
		ssatest.CheckSyntaxFlowContain(t, code,
			`request.getParameter() --> as $res`, map[string][]string{
				"res": {"rt.exec"},
			}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
