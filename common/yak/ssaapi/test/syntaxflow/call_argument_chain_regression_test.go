package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCallArgument_OutputFilterShouldNotCutMethodChain(t *testing.T) {
	code := `
import freemarker.template.*;
import java.io.*;
import java.util.*;

public class FreemarkerExample {
    public static void main(String[] args) {
        Configuration cfg = new Configuration(Configuration.VERSION_2_3_31);
        try {
            Template template = cfg.getTemplate("welcome.ftl");
            Map<String, Object> templateData = new HashMap<>();
            Writer out = new StringWriter();
            template.process(templateData, out);
        } catch (IOException | TemplateException e) {
            e.printStackTrace();
        }
    }
}
`

	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		tpl, err := prog.SyntaxFlowWithError(`getTemplate?{<typeName>?{have:"freemarker"}}(,*?{!opcode: const} as $sink) as $tpl`)
		require.NoError(t, err)
		require.Equal(t, 1, tpl.GetValues("tpl").Len())
		require.Equal(t, 0, tpl.GetValues("sink").Len())

		call, err := prog.SyntaxFlowWithError(`getTemplate?{<typeName>?{have:"freemarker"}}(,*?{!opcode: const} as $sink).process() as $call`)
		require.NoError(t, err)
		require.Equal(t, 1, call.GetValues("call").Len())
		require.Equal(t, 0, call.GetValues("sink").Len())

		params, err := prog.SyntaxFlowWithError(`getTemplate?{<typeName>?{have:"freemarker"}}(,*?{!opcode: const} as $sink).process(,* as $params,)`)
		require.NoError(t, err)
		require.Equal(t, 1, params.GetValues("params").Len())
		require.Equal(t, 0, params.GetValues("sink").Len())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
