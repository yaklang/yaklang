package java

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNativeCallTypeName(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		typeName := prog.SyntaxFlowChain(`documentBuilder<typeName> as $id;`, ssaapi.QueryWithEnableDebug())[0]
		assert.Contains(t, typeName.String(), "DocumentBuilder")
		typeName = prog.SyntaxFlowChain(`documentBuilder<fullTypeName> as $id;`, ssaapi.QueryWithEnableDebug())[0]
		assert.Contains(t, typeName.Show().String(), "javax.xml.parsers.DocumentBuilder")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestNativeCallTypeNameWithSCAVersion(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("FastJSONDemoController.java",
		`package com.example.demo.controller.fastjsondemo;

import com.alibaba.fastjson.JSON;
import org.apache.ibatis.annotations.Param;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/fastjson")
public class FastJSONDemoController {
    @GetMapping("/fromId")
    public ResponseEntity<Object> loadFromParam(@RequestParam(name = "id") String id) {
        // This is a FASTJSON Vuln typically.
        Object anyJSON = JSON.parse(id);
     
        return ResponseEntity.ok(anyJSON);
    }
}
`)
	vf.AddFile("pom.xml",
		`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>3.2.7</version>
        <relativePath/> <!-- lookup parent from repository -->
    </parent>
    <groupId>com.example</groupId>
    <artifactId>demo</artifactId>
    <version>0.0.1-SNAPSHOT</version>
    <name>demo</name>
    <description>Demo project for Spring Boot</description>
    <url/>
    <properties>
        <java.version>8</java.version>
    </properties>
    <dependencies>
        <dependency>
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <version>1.2.24</version>
        </dependency>
    </dependencies>
</project>
`)

	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		obj := prog.SyntaxFlowChain(`JSON<fullTypeName>?{have: 'alibaba.fastjson'} as $obj`, ssaapi.QueryWithEnableDebug()).Show(true)
		assert.NotNil(t, obj)

		obj = prog.SyntaxFlowChain(`parse*?{<getObject><fullTypeName>?{have: 'alibaba.fastjson'} } as $obj`, ssaapi.QueryWithEnableDebug()).Show(true)
		assert.NotNil(t, obj)

		obj = prog.SyntaxFlowChain(`ok()?{<getCallee><getObject><fullTypeName>?{have: 'org.springframework.'} } as $obj`, ssaapi.QueryWithEnableDebug()).Show(true)
		assert.NotNil(t, obj)

		typeName := prog.SyntaxFlowChain(`anyJSON<typeName>?{have:'JSON'} as $id;`, ssaapi.QueryWithEnableDebug()).Show()
		assert.Contains(t, typeName.String(), "JSON")
		typeName = prog.SyntaxFlowChain(`anyJSON<fullTypeName>?{have:'JSON'} as $id`, ssaapi.QueryWithEnableDebug())
		//TODO: fixup this SCA package version
		// assert.Contains(t, typeName.String(), "com.alibaba.fastjson.JSON:1.2.24")
		assert.Contains(t, typeName.String(), "com.alibaba.fastjson.JSON")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestLocalVariableDeclareTypeName(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java",
		`package com.org.LocalVariableDeclareTypeName.A;
				class A{
					};

		    `)
	vf.AddFile("B.java",
		`package com.example.LocalVariableDeclareTypeName.B;
			import com.org.LocalVariableDeclareTypeName.A.A;
			class B{
				public static void main(String[] args){
					A res1 = "aaa";  
					A res2 = 1;  				
					var res3 = A;
					var res4 ="a";     
					A res5 = Dog(); 
					A test1 ,test2 = Dog();
				}
			};	
	`)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()

		obj := prog.SyntaxFlowChain(`res1<typeName>?{have: 'string' || have: 'A'} as $obj`)
		assert.Equal(t, 3, obj.Len())
		obj = prog.SyntaxFlowChain(`res1<fullTypeName>?{have: 'string' || have: 'A'} as $obj`)
		assert.Equal(t, 2, obj.Len())

		obj = prog.SyntaxFlowChain(`res2<typeName>?{have:'number' || have: 'A'}  as $obj`)
		assert.Equal(t, 3, obj.Len())
		obj = prog.SyntaxFlowChain(`res2<fullTypeName>?{have:'number' || have: 'A'}as $obj`)
		assert.Equal(t, 2, obj.Len())

		obj = prog.SyntaxFlowChain(`res3<typeName>?{have:'A'} as $obj`)
		assert.Equal(t, 2, obj.Len())
		obj = prog.SyntaxFlowChain(`res3<fullTypeName>?{have:'com.org.LocalVariableDeclareTypeName.A.A'} as $obj`)
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain(`res4<typeName>?{have:'string'} as $obj`)
		assert.Equal(t, 1, obj.Len())
		obj = prog.SyntaxFlowChain(`res4<fullTypeName>?{have: 'string'}as $obj`)
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain(`res5<typeName>?{have:'A'}as $obj`)
		assert.Equal(t, 2, obj.Len())
		obj = prog.SyntaxFlowChain(`res5<fullTypeName>?{have:'com.org.LocalVariableDeclareTypeName.A.A'} as $obj`)
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain(`test2<typeName>?{have:'A'} as $obj`)
		assert.Equal(t, 2, obj.Len())
		obj = prog.SyntaxFlowChain(`test2<fullTypeName>?{have:'com.org.LocalVariableDeclareTypeName.A.A'} as $obj`)
		assert.Equal(t, 1, obj.Len())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}

func TestMemberCallTypeName(t *testing.T) {
	t.Run("membercall typename", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("Dog2.java", `package com.org.MemberCallTypeName.Dog; class Dog{};`)
		vf.AddFile("A.java",
			`package com.org.MemberCallTypeName.A;
			 import com.org.MemberCallTypeName.Dog.Dog;
				class A{
					public int existMethod(){return 666;}
					public Dog getDog(){return new Dog();}
					public static Dog staticMethod(){return new Dog();};
					};
		    `)
		vf.AddFile("B.java",
			`package com.example.MemberCallTypeName.B;
			import com.org.MemberCallTypeName.A.A;
			class B{
				public static void main(String[] args){
					A object = new A();
					var res1 = object.noExistMethod();  // fulltypeName 应该和object一样
					var res2 = object.existMethod();  // fulltypeName 应该为number
					var res3 = object.getDog();  // fulltypeName 应为com.org.Dog.Dog
					var res4 = object.method1().method2();  // fulltypeName 应该和object一样
					var res5 = A.staticMethod();  // fulltypeName 应该找到Dog
					var res6 = A.noExistMethod();  // fulltypeName 应该找到A	

					Runtime runtime = Runtime.getRuntime();
					runtime.exec();
				}
			};	
	`)

		ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			prog := progs[0]
			prog.Show()

			obj := prog.SyntaxFlowChain(`res1<typeName> as $obj`)
			assert.Equal(t, 2, obj.Len())
			obj = prog.SyntaxFlowChain(`res1<fullTypeName>?{have: 'com.org.MemberCallTypeName.A.A'} as $obj`)
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain(`res2<typeName>?{have:'int'} as $obj`)
			assert.Equal(t, 1, obj.Len())
			obj = prog.SyntaxFlowChain(`res2<fullTypeName>?{have:'int'} as $obj`)
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain(`res3<typeName>?{have:'Dog'} as $obj`)
			assert.Equal(t, 2, obj.Len())
			obj = prog.SyntaxFlowChain(`res3<fullTypeName>?{have:'com.org.MemberCallTypeName.Dog.Dog'} as $obj`)
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain(`res4<typeName>?{have:'A'} as $obj`)
			assert.Equal(t, 2, obj.Len())
			obj = prog.SyntaxFlowChain(`res4<fullTypeName>?{have: 'com.org.MemberCallTypeName.A.A'}as $obj`)
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain(`res5<typeName>?{have:'Dog'} as $obj`)
			assert.Equal(t, 2, obj.Len())
			obj = prog.SyntaxFlowChain(`res5<fullTypeName>?{have: 'com.org.MemberCallTypeName.Dog.Dog'}as $obj`)
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain(`res6<typeName>?{have:'A'} as $obj`)
			assert.Equal(t, 2, obj.Len())
			obj = prog.SyntaxFlowChain(`res6<fullTypeName>?{have: 'com.org.MemberCallTypeName.A.A'}as $obj`)
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain(`runtime<typeName>?{have:'com.example.MemberCallTypeName.B.Runtime'} as $obj`)
			assert.Equal(t, 1, obj.Len())
			obj = prog.SyntaxFlowChain(`.exec<typeName>?{have:'com.example.MemberCallTypeName.B.Runtime'} as $obj`)
			assert.Equal(t, 1, obj.Len())

			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestParamTypeName(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java",
		`package com.org.ParamTypeName.A;
				class A{
					};
		    `)
	vf.AddFile("B.java",
		`package com.example.ParamTypeName.B;
			import com.org.ParamTypeName.A.A;
			class B{
				public void hello(int param1,A param2,Dog param3){
					var res1 = param1;
					var res2 = param2;
					var res3 = param3; //Dog()为找不到的类，使用自身作为fullTypeName
					var res4 = a;
				}
			};	
	`)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()

		obj := prog.SyntaxFlowChain(`param1<typeName>?{have: 'int'} as $obj`)
		assert.Equal(t, 1, obj.Len())
		obj = prog.SyntaxFlowChain(`param1<fullTypeName>?{have: 'int'} as $obj`)
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain(`param2<typeName>?{have:'A'} as $obj`)
		assert.Equal(t, 2, obj.Len())
		obj = prog.SyntaxFlowChain(`param2<fullTypeName>?{have:'com.org.ParamTypeName.A.A'} as $obj`)
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain(`param3<typeName>?{have:'Dog'} as $obj`)
		assert.Contains(t, obj.String(), "Dog", "com.example.ParamTypeName.B.Dog")
		obj = prog.SyntaxFlowChain(`param3<fullTypeName>?{have:'Dog'} as $obj`)
		assert.Contains(t, obj.String(), "com.example.ParamTypeName.B.Dog")

		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestTypeMergeWithUndefinedAndByteArray(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java",
		`public class Jdbc {

    /**
     * <a href="https://github.com/JoyChou93/java-sec-code/wiki/CVE-2022-21724">CVE-2022-21724</a>
     */ 
    @RequestMapping("/postgresql")
    public void postgresql(String jdbcUrlBase64) throws Exception{
        byte[] b = java.util.Base64.getDecoder().decode(jdbcUrlBase64);
        String jdbcUrl = new String(b);
        log.info(jdbcUrl);
        DriverManager.getConnection(jdbcUrl);
    }

    private String getImgBase64(String imgFile) throws IOException {
        File f = new File(imgFile);
        byte[] data = Files.readAllBytes(Paths.get(imgFile)); //FIXME: this not match 
        return new String(Base64.encodeBase64(data));
    }
}
		    `)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()

		obj := prog.SyntaxFlowChain(`String() as $string_constructor 
$string_constructor?{<getActualParams()>* <fullTypeName()><var("aaa")>?{have:"byte"}} as $target `)
		assert.Contains(t, obj.String(), "byte")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestTypeNamePriority(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java",
		`package com.org.Priority.A;
				class A{
					};
		    `)
	vf.AddFile("B.java",
		`package com.example.Priority.B;
			import com.org.Priority.A.A;
			class B{
				public void hello(int param1,A param2){
					Object res1 = (A)param1;
					A res2 = (int)param2;
				}
			};	
	`)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()

		obj := prog.SyntaxFlowChain(`res1<typeName>?{have: 'A'} as $obj`)
		assert.Equal(t, 2, obj.Len())
		obj = prog.SyntaxFlowChain(`res1<fullTypeName>?{have: 'com.org.Priority.A'} as $obj`)
		assert.Equal(t, 1, obj.Len())
		obj = prog.SyntaxFlowChain(`res2<typeName>?{have:'int'} as $obj`)
		assert.Equal(t, 1, obj.Len())
		obj = prog.SyntaxFlowChain(`res2<fullTypeName>?{have:'int'} as $obj`)
		assert.Equal(t, 1, obj.Len())

		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestTypeNameForImportStar(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java",
		`package com.org.ImportStar.A;
				class A{
					};
		    `)
	vf.AddFile("B.java",
		`package com.example.ImportStar.B;
			import com.org.ImportStar.A.A;
			import com.yak.ImportStar.*;
			class B{
				public void hello(int param1,Dog param2){
					var res1 = param2; 
					Cat res2 = Cat();
					var res3 = new Cat();
				}
			};	
	`)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()

		typeName := prog.SyntaxFlowChain(`res1<typeName> as $id;`)
		assert.Contains(t, typeName.String(), "com.yak.ImportStar.Dog", "com.example.ImportStar.B.Dog", "Dog")
		typeName = prog.SyntaxFlowChain(`res1<fullTypeName> as $id;`)
		assert.Contains(t, typeName.String(), "com.yak.ImportStar.Dog", "com.example.ImportStar.B.Dog")

		typeName = prog.SyntaxFlowChain(`res2<typeName> as $id;`)
		assert.Contains(t, typeName.String(), "com.yak.ImportStar.Cat", "com.example.ImportStar.B.Cat", "Cat")
		typeName = prog.SyntaxFlowChain(`res2<fullTypeName> as $id;`)
		assert.Contains(t, typeName.String(), "com.yak.ImportStar.Cat", "com.example.ImportStar.B.Cat")

		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestFullTypeNameWithParentClass1(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("C.java", `
	package com.ParentClass1.yak;
	class C{};
	`)
	vf.AddFile("A.java",
		`package com.org.ParentClass1.A;
			class A {
				};
		`)
	vf.AddFile("B.java",
		`package com.example.ParentClass1.B;
		import com.org.ParentClass1.A.A;
		import com.ParentClass1.yak.C;
		class B extends A implements C{
			public static void main(String[] args){
				B a = new B();
			}
		};	
`)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()
		flow := prog.SyntaxFlow(`a<fullTypeName><show>`, ssaapi.QueryWithEnableDebug())
		flow.Show()
		obj := prog.SyntaxFlowChain("a<typeName> as $obj")
		assert.Equal(t, 6, obj.Len())

		obj = prog.SyntaxFlowChain("a<typeName>?{have:'com.example.ParentClass1.B'} as $obj")
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain("a<typeName>?{have:'com.org.ParentClass1.A.A'} as $obj")
		obj.Show()
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain("a<typeName>?{have:'com.org.ParentClass1.A.A'} as $obj")
		obj.Show()
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain("a<fullTypeName>?{have:'com.ParentClass1.yak.C'}as $obj")
		obj.Show()
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain("a<fullTypeName>?{have:'com.example.ParentClass1.B'} as $obj")
		obj.Show()
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain("a<fullTypeName>?{have:'com.org.ParentClass1.A.A'} as $obj")
		obj.Show()
		assert.Equal(t, 1, obj.Len())

		obj = prog.SyntaxFlowChain("a<typeName>?{have:'com.org.ParentClass1.A.A'} as $obj")
		assert.Equal(t, 1, obj.Len())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestFullTypeNameWithParentClass2(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("B.java",
		`package com.example.ParentClass2.B;
	
		class B extends A implements C{
			public static void main(String[] args){
				var a = new B();
			}
		};	
`)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()

		obj := prog.SyntaxFlowChain("a<typeName> as $obj").Show()
		assert.Contains(t, obj.String(), "com.example.ParentClass2.B.B", "B")
		assert.Contains(t, obj.String(), "com.example.ParentClass2.B.A", "A")
		assert.Contains(t, obj.String(), "com.example.ParentClass2.B.C", "C")

		obj = prog.SyntaxFlowChain("a<fullTypeName> as $obj").Show()
		assert.Contains(t, obj.String(), "com.example.ParentClass2.B.B")
		assert.Contains(t, obj.String(), "com.example.ParentClass2.B.A")
		assert.Contains(t, obj.String(), "com.example.ParentClass2.B.C")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestFullTypeNameForAnnotation(t *testing.T) {
	t.Run("test spring framework annotation", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("Test.java", `
	package com.Annotation1.example;
import org.springframework.web.bind.annotation.*;
import jakarta.servlet.http.HttpServletRequest;
@RequestMapping("/fastjson")
public class FastJSONDemoController {

    public ResponseEntity<Object> loadFromParam(@RequestParam(name = "id") int id) {

    }
}`)

		ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			prog := progs[0]
			prog.Show()
			obj := prog.SyntaxFlowChain("id.annotation.RequestParam<fullTypeName>?{have:'org.springframework.web.bind.annotation.RequestParam'} as $obj")
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain("FastJSONDemoController.annotation.RequestMapping<fullTypeName>?{have:'org.springframework.web.bind.annotation.RequestMapping'} as $obj")
			assert.Equal(t, 1, obj.Len())

			obj = prog.SyntaxFlowChain("*Param.__ref__<fullTypeName>?{have:int} as $obj")
			assert.Equal(t, 1, obj.Len())

			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test no spring framework anntation type name ", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("Test.java", `
package com.Annotation2.example;
import org.springframework.web.bind.annotation.*;
import jakarta.servlet.http.HttpServletRequest;

public class FastJSONDemoController {

    public ResponseEntity<Object> loadFromParam(@Hello(name = "id") int id) {

    }
}`)

		ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			prog := progs[0]
			prog.Show()
			obj := prog.SyntaxFlowChain("id.annotation.Hello<fullTypeName> as $obj").Show()
			//assert.Equal(t, 2, obj.Len())
			require.Contains(t, obj.String(), "com.Annotation2.example.Hello")
			require.Contains(t, obj.String(), "java.lang.Hello")
			require.Contains(t, obj.String(), "org.springframework.web.bind.annotation.Hello")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test servlet annotation1", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("Test.java", `
	package com.Annotation3.example;

import javax.servlet.annotation.*; 
@WebServlet(value = "/Simple") 
public class Simple extends HttpServlet {

   private static final long serialVersionUID = 1L; 

   protected void doGet(HttpServletRequest request, HttpServletResponse response)  
       { 
   }   
}`)

		ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			prog := progs[0]
			prog.Show()
			obj := prog.SyntaxFlowChain("Simple.annotation.WebServlet<fullTypeName>?{have:'javax.servlet.annotation.WebServlet'} as $obj")
			assert.Equal(t, 1, obj.Len())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test servlet annotation2", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("Test.java", `
	package com.Annotation4.example;

import javax.servlet.annotation.WebServlet; 
@WebServlet(value = "/Simple") 
public class Simple extends HttpServlet {

   private static final long serialVersionUID = 1L; 

   protected void doGet(HttpServletRequest request, HttpServletResponse response)  
       { 
   }   
}`)

		ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
			prog := progs[0]
			prog.Show()
			obj := prog.SyntaxFlowChain("Simple.annotation.WebServlet<fullTypeName>?{have:'javax.servlet.annotation.WebServlet'} as $obj")
			assert.Equal(t, 1, obj.Len())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestTypeNameForCreator(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		code := `package com.Annotation5.example;
		import java.io.FileWriter;
		import java.io.File;
		class A{
			public static main(String[] args){
			FileWriter fw = new FileWriter(new File("a.txt"));
			}
		}`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			res := prog.SyntaxFlowChain(`File<typeName>?{have:'File'} as $a;`)
			assert.Equal(t, 2, res.Len())

			res = prog.SyntaxFlowChain(`File<fullTypeName>?{have:'java.io.File'} as $a;`)
			assert.Equal(t, 1, res.Len())

			res = prog.SyntaxFlowChain(`FileWriter<typeName>?{have:'FileWriter'} as $a;`)
			assert.Equal(t, 2, res.Len())
			res = prog.SyntaxFlowChain(`FileWriter<typeName>?{have:'java.io.FileWriter'} as $a;`)
			assert.Equal(t, 1, res.Len())

			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test chain creator simple", func(t *testing.T) {
		code := `
	import okhttp3.Request;
	class Main{
	public static void main(String[] args) {
		Request request = new Request.Builder();
	    }
	}
`
		ssatest.CheckSyntaxFlowContain(t, code, `Request?{<typeName>?{have:'okhttp3.'}}.Builder as $result`, map[string][]string{
			"result": {"Undefined-Builder(valid)"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test chain creator complex", func(t *testing.T) {
		code := `
	import okhttp3.Request;
	class Main{
	public static void main(String[] args) {
		Request request = new Request.Builder.AAA.BBB();
	    }
	}
`
		ssatest.CheckSyntaxFlowContain(t, code, `Request.Builder.AAA<typeName> as $result`, map[string][]string{
			"result": {"okhttp3.Request"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestNativeCall_Forbid(t *testing.T) {
	ssatest.Check(t, `
a = (b) => {
	return b + 1;
}
c = a();
`, func(prog *ssaapi.Program) error {
		_, err := prog.SyntaxFlowWithError("b<show><forbid>")
		if err != nil && errors.Is(err, sfvm.CriticalError) && strings.Contains(err.Error(), "forbid") {
			return nil
		}
		return utils.Errorf("forbid native call is not finished: %v", err)
	})
}

func TestNativeCall_Forbid2(t *testing.T) {
	ssatest.Check(t, `
a = (b) => {
	return b + 1;
}
c = a();
`, func(prog *ssaapi.Program) error {
		_, err := prog.SyntaxFlowWithError("b<show> as $ccccc; <forbid(ccccc)>")
		if err != nil && errors.Is(err, sfvm.CriticalError) && strings.Contains(err.Error(), "forbid") {
			return nil
		}
		return utils.Error("forbid native call is not finished")
	})
}

func TestNativeCall_Java_ReturnType(t *testing.T) {
	code := `
class Main{
	public String foo(string param){
		return param;
	}
	public Integer bar(){
		return xxx;
	}
	public Long test(){
		return call();
	}
}
`
	ssatest.CheckSyntaxFlowContain(t, code, `foo<getReturns><typeName> as $f; bar<getReturns><typeName> as $b;
	test<getReturns><typeName> as $t;`,
		map[string][]string{
			"f": {"\"String\""},
			"b": {"\"Integer\""},
			"t": {"\"Long\""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA),
	)
}
func TestReturnType(t *testing.T) {
	code := `
class Main{
	public Long test(){
		return call();
	}
}`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		vals, err := prog.SyntaxFlowWithError(`test<getReturns><typeName> as $b`)
		assert.NoError(t, err)
		b := vals.GetValues("b")
		assert.Contains(t, b.String(), "Long")
		assert.Contains(t, b.String(), "java.lang.Long")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func Test_Class_Declare_Type_Name(t *testing.T) {
	code := `
package com.mycompany.myapp;

import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import java.util.List;

@Mapper
public interface UserMapper {

    User getUser(@Param("id") Long id);

    void insertUser(User user);

    void updateUser(User user);

    void deleteUser(@Param("id") Long id);

    List<User> getAllUsers(); 
}
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		vals, err := prog.SyntaxFlowWithError(`UserMapper_declare<typeName> as $res`)
		require.NoError(t, err)
		res := vals.GetValues("res")
		res.Show()
		require.Contains(t, res.String(), "UserMapper")
		require.Contains(t, res.String(), "com.mycompany.myapp.UserMapper")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestJava_Creator_MethodCallTypeName(t *testing.T) {
	t.Run("test new method call type name", func(t *testing.T) {
		code := `import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;

public class OkHttpClientExample {
    public static void main(String[] args) {
        OkHttpClient client = new OkHttpClient();
        Request request = new Request.Builder()
                .url("https://api.github.com/users/github")
                .build();
        try {
            // 执行请求
            Response response = client.newCall(request).execute();
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}`
		ssatest.CheckSyntaxFlowContain(t, code, `Request.Builder()<typeName> as $result1;
Request.Builder().url()<typeName> as $result2`, map[string][]string{
			"result1": {"okhttp3.Request"},
			"result2": {"okhttp3.Request"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestJavaValueOnlyDeclareTypeName(t *testing.T) {
	t.Run("only declare typename simple declare simple", func(t *testing.T) {
		code := `
package org.joychou.controller;

public class ClassDataLoader {
    public void classData() {
		java.lang.reflect.Method defineClassMethod;
    }
}
	`

		ssatest.CheckSyntaxFlowContain(t, code, `
defineClassMethod<fullTypeName()> as $name
	`, map[string][]string{
			"name": {"\"java.lang.reflect.Method\""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("full typename only declare", func(t *testing.T) {
		t.Skip("skip for now, need to fixup the java parser")
		code := `
package org.joychou.controller;

public class ClassDataLoader {
    public void classData() {
		Method defineClassMethod;
    }
}
	`

		ssatest.CheckSyntaxFlow(t, code, `
defineClassMethod<fullTypeName()> as $name
	`, map[string][]string{
			"name": {"\"java.lang.Method\"", "\"org.joychou.controller.Method\""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
