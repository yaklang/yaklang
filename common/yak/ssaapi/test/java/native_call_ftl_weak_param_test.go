package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

func TestNativeCall_FreeMarkerXSS(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("com/example/demo/controller/FreeMarkerdemo/FreeMarkerDemo.java", `package com.example.demo.controller.FreeMarkerdemo;
    
@Controller
@RequestMapping("/freemarker")
public class FreeMarkerDemo {
    

    @GetMapping("/welcome")
    public String welcome(@RequestParam String name, Model model) {
        if (name == null || name.isEmpty()) {
            model.addAttribute("name", "Welcome to Safe FreeMarker Demo, try <code>/freemarker/safe/welcome?name=Hacker<>");
        } else {
            model.addAttribute("name", name);
        }
        return "welcome";
    }

}`)
	vf.AddFile("src/main/resources/application.properties", `spring.application.name=demo
# FreeMarker
spring.freemarker.template-loader-path=classpath:/templates/
spring.freemarker.suffix=.ftl
`)
	vf.AddFile("welcome.ftl", `<!DOCTYPE html>
<html>
<head>
    <title>Welcome</title>
</head>
<body>
<h1>Welcome ${name1}!</h1>
<h1>${name2?html}!</h1>
</body>
</html>
`)
	t.Run("FreeMarkerXSS", func(t *testing.T) {
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			sink, err := prog.SyntaxFlowWithError(`
		*Mapping.__ref__ as $ref 
		$ref <getFunc><getReturns> as $ret 
		$ret ?{<typeName>?{have:'String'}} as $target 
		$target <freeMarkerSink>  as  $a
		`)
			sink.Show()
			require.NoError(t, err)
			assert.Equal(t, 1, sink.GetValues("a").Len())
			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
}

func TestNativeCall_FreeMarkerXSS_WithNoSuffixConfig(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("com/example/demo/controller/FreeMarkerdemo/FreeMarkerDemo.java", `package com.example.demo.controller.FreeMarkerdemo;
    

@Controller
@RequestMapping("/freemarker")
public class FreeMarkerDemo {
    @Autowired
    private Configuration freemarkerConfig;

    @GetMapping("/welcome")
    public String welcome(@RequestParam String name, Model model) {
        if (name == null || name.isEmpty()) {
            model.addAttribute("name", "Welcome to Safe FreeMarker Demo, try <code>/freemarker/safe/welcome?name=Hacker<>");
        } else {
            model.addAttribute("name", name);
        }
        return "welcome.ftl";
    }

}`)
	vf.AddFile("welcome.ftl", `<!DOCTYPE html>
<html>
<head>
    <title>Welcome</title>
</head>
<body>
<h1>Welcome ${name}!</h1>
</body>
</html>
`)

	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		sink := prog.SyntaxFlowChain("*Mapping.__ref__<getFunc><getReturns>?{<typeName>?{have:'String'}}<freeMarkerSink>  as  $a")
		assert.Equal(t, 1, sink.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestNativeCall_FreeMarkerXSS_WithDirfferentSuffix(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("com/example/demo/controller/FreeMarkerdemo/FreeMarkerDemo.java", `package com.example.demo.controller.FreeMarkerdemo;
    

@Controller
@RequestMapping("/freemarker")
public class FreeMarkerDemo {
    @Autowired
    private Configuration freemarkerConfig;

    @GetMapping("/welcome")
    public String welcome(@RequestParam String name, Model model) {
        if (name == null || name.isEmpty()) {
            model.addAttribute("name", "Welcome to Safe FreeMarker Demo, try <code>/freemarker/safe/welcome?name=Hacker<>");
        } else {
            model.addAttribute("name", name);
        }
    return "welcome";
    }

}`)
	vf.AddFile("src/main/resources/application.properties", `spring.application.name=demo
# FreeMarker
spring.freemarker.template-loader-path=classpath:/templates/
spring.freemarker.suffix=.html
`)
	vf.AddFile("welcome.html", `<!DOCTYPE html>
<html>
<head>
    <title>Welcome</title>
</head>
<body>
<h1>Welcome ${name}!</h1>
</body>
</html>
`)

	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		sink := prog.SyntaxFlowChain("*Mapping.__ref__<getFunc><getReturns>?{<typeName>?{have:'String'}}<freeMarkerSink>  as  $a")
		assert.Equal(t, 1, sink.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
