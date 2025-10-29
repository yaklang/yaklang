package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}

func TestArrayType(t *testing.T) {
	byteType := ssa.CreateByteType()
	byteType.AddFullTypeName("byte")

	generatedType := java2ssa.TypeAddBracketLevel(byteType, 0)
	require.ElementsMatch(t, generatedType.GetFullTypeNames(), []string{"byte"})

	generatedType = java2ssa.TypeAddBracketLevel(byteType, 1)
	require.ElementsMatch(t, generatedType.GetFullTypeNames(), []string{"byte[]"})

	generatedType = java2ssa.TypeAddBracketLevel(byteType, 2)
	require.ElementsMatch(t, generatedType.GetFullTypeNames(), []string{"byte[][]"})

	generatedType = java2ssa.TypeAddBracketLevel(byteType, 3)
	require.ElementsMatch(t, generatedType.GetFullTypeNames(), []string{"byte[][][]"})

	objSlice, ok := ssa.ToObjectType(generatedType)
	require.True(t, ok)
	require.ElementsMatch(t, objSlice.FieldType.GetFullTypeNames(), []string{"byte[][]"})

	objSlice1, ok := ssa.ToObjectType(objSlice.FieldType)
	require.True(t, ok)
	require.ElementsMatch(t, objSlice1.FieldType.GetFullTypeNames(), []string{"byte[]"})

	objSlice2, ok := ssa.ToObjectType(objSlice1.FieldType)
	require.True(t, ok)
	require.ElementsMatch(t, objSlice2.FieldType.GetFullTypeNames(), []string{"byte"})
}
