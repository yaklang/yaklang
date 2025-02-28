package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
<include('java-spring-param')>?{<typeName>?{have: String}} as $params;
RestController.__ref__<getMembers>?{.annotation.*Mapping} as $entryMethods;
$entryMethods<getReturns> as $sink;
$sink<show> #{
    until: <<<CODE
* & $params as $source
CODE,
}->;
$source<show>
$source<dataflow(<<<CODE
*?{opcode: call && <getCallee><name><show><isSanitizeName>} as $__next__
CODE)> as $haveCall;
alert $haveCall;
`)
		assert.Equal(t, results.GetValues("haveCall").Len(), 1)
		assert.Equal(t, results.GetValues("sink").Len(), 2)
		return nil
	})
}
