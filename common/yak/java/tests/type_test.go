package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestTypeLeftTypeAndRightType(t *testing.T) {
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

	ssatest.CheckSyntaxFlow(t, code, `
PrintWriter as $writer 
$writer.write(, * as $text) as $write_site
	`, map[string][]string{
		"text": {`"aaaa"`},
	}, ssaapi.WithLanguage(consts.JAVA))

}
