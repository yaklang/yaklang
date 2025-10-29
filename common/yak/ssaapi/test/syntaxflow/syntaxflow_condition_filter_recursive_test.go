package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSyntaxFlowConditionBad(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("A.java", `package com.example.demo.controller.fastjsondemo1;

import com.alibaba.fastjson.JSON;
import org.apache.ibatis.annotations.Param;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import jakarta.servlet.http.HttpServletRequest;

@RestController
@RequestMapping("/fastjson")
public class FastJSONDemoController {

	@Autowired
    private HttpServletRequest request;

	@GetMapping("/fromId")
    public ResponseEntity<Object> loadFromParam(@RequestParam(name = "id") String id) {
        // This is a FASTJSON Vuln typically.
        Object anyJSON = JSON.parse(id);

		request.getParameter("id");
        return ResponseEntity.ok(anyJSON);
    }
}`)
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		results := prog.SyntaxFlowChain(`.getParameter()?{<getCallee><getObject><fullTypeName>?{have: servlet} || <getFunc>.annotation.*Mapping} as $dynamicParams`)
		assert.Equal(t, 1, len(results))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}
