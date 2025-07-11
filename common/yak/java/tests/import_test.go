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

func TestImportClassTypeName(t *testing.T) {
	t.Run("check import type with import name", func(t *testing.T) {

		code := `
package com.example.fastjsondemo.controller;

import com.alibaba.fastjson.JSON;
import com.example.fastjsondemo.model.User;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api")
public class UserController {

    @PostMapping("/user")
    public User createUser(@RequestBody String jsonString) {
        // 使用 FastJSON 将 JSON 字符串解析为 User 对象
        User user = JSON.parseObject(jsonString, User.class);

		Int b = JSON;


    }
}
	`

		ssatest.CheckSyntaxFlowContain(t, code, `

// check json.parse
JSON.parse* as $parse 
$parse<getObject> as $json 
$json<typeName> as $typeName;

// check assign 
b<typeName> as $jsonType2

	`, map[string][]string{
			"typeName":  {"com.alibaba.fastjson.JSON"},
			"jsonType2": {"com.alibaba.fastjson.JSON"},
		}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("check import type with import start", func(t *testing.T) {
		code := `
package com.example.fastjsondemo.controller;

import com.alibaba.fastjson.*;
import com.example.fastjsondemo.model.User;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api")
public class UserController {

    @PostMapping("/user")
    public User createUser(@RequestBody String jsonString) {
        // 使用 FastJSON 将 JSON 字符串解析为 User 对象
        User user = JSON.parseObject(jsonString, User.class);

		Object b = JSON;


    }
}
		`

		ssatest.CheckSyntaxFlowContain(t, code, `

// check json.parse
JSON.parse* as $parse 
$parse<getObject> as $json 
$json<typeName> as $typeName;

// check assign 
b<typeName> as $jsonType2

	`, map[string][]string{
			"typeName":  {"com.alibaba.fastjson.JSON"},
			"jsonType2": {"com.alibaba.fastjson.JSON"},
		}, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("check import type with creator", func(t *testing.T) {
		code := `
		package org.example;
		import okhttp3.OkHttpClient;
		import okhttp3.Request;
		import okhttp3.Response;
		
		
		public class OkHttpClientExample {
			public static void main(String[] args) {
				Request request = new Request.Builder()
						.url("https://api.github.com/users/github")
						.build();
			}
		}
		
		`

		ssatest.CheckSyntaxFlowContain(t, code, `
		Request.Builder<getObject><typeName>  as $request_type_name 
		Request.Builder<typeName>  as $builder_type_name
		Request.Builder()<typeName> as $builder_constructor_type_name
		`, map[string][]string{
			"request_type_name":             {"okhttp3.Request"},
			"builder_type_name":             {"okhttp3.Request.Builder"},
			"builder_constructor_type_name": {"okhttp3.Request.Builder"},
		}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("check import type with full name", func(t *testing.T) {
		code := `
		public class OkHttpClientExample {
			@RequestMapping(value = "/three")
			public String Three(@RequestParam(value = "url") String imageUrl) {
				com.squareup.okhttp.Request request = new com.squareup.okhttp.Request.Builder().get().url(url).build();
			}
		}
				`
		ssatest.CheckSyntaxFlowContain(t, code, `
				Request.Builder as $builder 
				Request.Builder<getObject><typeName>  as $request_type_name
				Builder<typeName>  as $builder_type_name 
				`, map[string][]string{
			"builder":           {"Undefined-Builder(valid)"},
			"request_type_name": {"com.squareup.okhttp.Request"},
			"builder_type_name": {"com.squareup.okhttp.Request.Builder"},
		}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("check import type same name with current package ", func(t *testing.T) {
		code := `
		package com.ruoyi.web.controller.common;
	
		import com.ruoyi.common.utils.file.FileUtils;
	
	public class FileUtils
	{
		public static void writeBytes(String filePath, OutputStream os) throws IOException{}
	}
	
		@Controller
		public class CommonController
		{
			private static final Logger log = LoggerFactory.getLogger(CommonController.class);
	
			@Autowired
			private ServerConfig serverConfig;
	
			/**
			 * 通用下载请求
			 *
			 * @param fileName 文件名称
			 * @param delete 是否删除
			 */
			@GetMapping("common/download")
			public void fileDownload(String fileName, Boolean delete, HttpServletResponse response, HttpServletRequest request)
			{
				FileUtils.writeBytes(filePath, response.getOutputStream());
			}
		}
		`

		ssatest.CheckSyntaxFlowSource(t, code, `
	filePath?{opcode: param}<getFunc> as $function 
	$function() as $function_call_site
	`, map[string][]string{
			"function_call_site": {"writeBytes(filePath, response.getOutputStream())"},
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}
