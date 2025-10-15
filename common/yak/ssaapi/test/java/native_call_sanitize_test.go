package java

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSanitizeNameFetching(t *testing.T) {
	ssatest.CheckJava(t, `
import org.springframework.web.bind.annotation.*;
import org.springframework.web.servlet.ModelAndView;
import org.springframework.web.util.HtmlUtils;

@RestController
@RequestMapping("/xss")
public class XSSController {
    @PostMapping("/submit")
    public String handleSubmit(@RequestParam("userInput") String userInput) {
		userInput = xssEscapeFree(userInput);
        return "处理后的输入: " + userInput;
    }
    @PostMapping("/submit2")
    public String handleSubmit2(@RequestParam("userInput1") String unsafe) {
		userInput = nosuchiii(userInput);
        return "处理后的输入: " + userInput;
    }
}
`, func(prog *ssaapi.Program) error {
		results := prog.SyntaxFlow(`

*Mapping.__ref__?{opcode:function}<getFormalParams>?{!have:this} as $params
RestController.__ref__<getMembers>?{.annotation.*Mapping} as $entryMethods;
$entryMethods<getReturns> as $sink;
$sink #{
	include: <<<CODE
* & $params as $source
CODE,
	include: <<<CODE
*?{opcode: call && <getCallee><name><show><isSanitizeName>} as $haveCall
CODE
		}-> 
		`, ssaapi.QueryWithEnableDebug())
		results.Show()
		require.Equal(t, results.GetValues("sink").Len(), 2)
		require.Equal(t, results.GetValues("haveCall").Len(), 1)
		return nil
	})
}
