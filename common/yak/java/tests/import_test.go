package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public  int get() {
			return 	 1;
		}
	}
	`)
	vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			A a = new A();
			println(a.get());
		}
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1"},
	}, false, ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestImport_FilePath(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/io/github/talelin/latticy/common/aop/ResultAspect.java", `package io.github.talelin.latticy.common.aop;
import io.github.talelin.latticy.module.log.MDCAccessServletFilter;

@Aspect
@Component
public class ResultAspect {
    public void doAfterReturning(UnifyResponseVO<String> result) {
    }
}
`)

	vf.AddFile(`src/main/java/io/github/talelin/latticy/module/log/MDCAccessServletFilter.java`, `
package io.github.talelin.latticy.module.log;

public class MDCAccessServletFilter implements Filter {
    @Override
    public void doFilter(ServletRequest request, ServletResponse response, FilterChain chain) throws IOException, ServletException {
    }
}
`)
	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		result, err := prog.SyntaxFlowWithError(`doFilter`)
		if err != nil {
			return err
		}
		log.Info(result.String())
		require.Contains(t, result.String(), "src/main/java/io/github/talelin/latticy/module/log/MDCAccessServletFilter.java")
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
func TestImportWithInterface(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/A.java", `
package src.main.java;
public interface HomeDao {
    List<PmsBrand> getRecommendBrandList(@Param("offset") Integer off,@Param("limit") Integer limit);
}`)
	vf.AddFile("src/main/org/B.java", `
package src.main.org;
import src.main.java.HomeDao;
class A{
	@Autowired
    private HomeDao homeDao;
	public void BB(){
		homeDao.getRecommendBrandList(1,2);
}
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf,
		`off #-> * as $param`,
		map[string][]string{
			"param": {"1"},
		}, false, ssaapi.WithLanguage(ssaapi.JAVA))
}
func TestImportClass(t *testing.T) {
	t.Run("import class", func(t *testing.T) {
		fs := filesys.NewVirtualFs()
		fs.AddFile("com/example/demo1/A.java", `
package com.example.demo1;
class A {
    public static int a = 1;
	public static void test(){
		return 1;
	}
}
`)
		fs.AddFile("com/example/demo2/test.java", `
package com.example.demo2;
import com.example.demo1.A;
class test {
    public static void main(String[] args) {
        println(A.test());
    }
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* #-> * as $param)`, map[string][]string{"param": {"1"}}, false, ssaapi.WithLanguage(ssaapi.JAVA))
	})
	t.Run("import class2", func(t *testing.T) {
		//todo: the case need default ???
		fs := filesys.NewVirtualFs()
		fs.AddFile("a.java", `
package com.example.demo1;
import com.example.demo.B;
class A{
	public B b;
	public void main(){
		println(this.b.a);
	}
}

`)
		fs.AddFile("b.java", `
package com.example.demo;
class B{
	public static int a = 1;
}
`)
		ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* as $sink)`,
			map[string][]string{
				"sink": {"ParameterMember-parameterMember"},
			},
			true,
			ssaapi.WithLanguage(ssaapi.JAVA))
	})
}

func TestImportStaticAll(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a.java", `
package com.example.demo2;

public class A {
    public static int a = 1;

    public static void Method(int b) {
		println(b);
    }
}
`)
	fs.AddFile("b.java", `
package com.example.demo1;

import static com.example.demo2.A.*;

class A {
	public static void main(){
		Method(1);
		println(a);
	}
}`)
	ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* #-> * as $param)`,
		map[string][]string{
			"param": {"1", "1"},
		},
		true,
		ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestImportStaticMember(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a.java", `
package com.example.demo2;

public class A {
    public static int a = 1;

    public static void Method(int a) {
		println(a);
    }
}
`)
	fs.AddFile("b.java", `
package com.example.demo1;

import static com.example.demo2.A.a;

class A {
	public static void main(){
		println(a);
		Method(1);
	}
}`)
	ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* #-> * as $param)`,
		map[string][]string{
			"param": {"1", "Parameter-a"},
		},
		true,
		ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestImportSourceCodeRange(t *testing.T) {
	code := `
	package com.example.demo.controller.freemakerdemo;

import java.io.IOException;
import java.io.PrintWriter;

@Controller
@RequestMapping("/freemarker")
public class FreeMakerDemo {
    @Autowired
    private Configuration freemarkerConfig;

    @GetMapping("/template")
    public void template(String name, Model model, HttpServletResponse response) throws Exception {
        PrintWriter writer = response.getWriter();
        writer.write("aaaa");
        writer.flush();
        writer.close();
    }
}
	`

	ssatest.CheckSyntaxFlowSource(t, code, `
PrintWriter as $writer
	`, map[string][]string{
		"writer": {"import java.io.PrintWriter;", "getWriter()"},
	}, ssaapi.WithLanguage(consts.JAVA))
}
