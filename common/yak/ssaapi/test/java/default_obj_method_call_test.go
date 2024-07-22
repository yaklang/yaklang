package java

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

const DefaultOBJMethodCall = `package com.example.demo.controller.deepcross;
@RestController
public class DeepCrossController {
    @GetMapping({"/xss/direct/6"})
    public ResponseEntity<String> noDeepCross6(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
        body = body.replaceAll("Hello", "---Hello---");
        body += "\n\nSigned by DeepCrossController";
        body = DummyUtil.filterXSS(body);
        ResponseEntity<String> resp = new ResponseEntity(body, HttpStatus.OK);
        return resp;
    }
}

`

func TestDefaultOBJMethodCall(t *testing.T) {
	ssatest.Check(t, DefaultOBJMethodCall, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlow(`.replaceAll(*?{!opcode: const} as $param,) as $sink; check $param`)
		if result.GetValues("param").Len() <= 0 {
			t.Fatal("replaceAll bind object not found")
		}
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
