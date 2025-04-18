package syntaxflow

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestLib_SpringControllerParam(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("A.java", `package com.example.demo.controller.fastjsondemo1;

import com.alibaba.fastjson.JSON;
import org.apache.ibatis.annotations.Param;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/fastjson")
public class FastJSONDemoController {
    @GetMapping("/fromId")
    public ResponseEntity<Object> loadFromParam(@RequestParam(name = "id") String id) {
        // This is a FASTJSON Vuln typically.
        Object anyJSON = JSON.parse(id);
        return ResponseEntity.ok(anyJSON);
    }
}`)
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		results := prog.SyntaxFlowChain(`<include('java-spring-mvc-param')> as $params`)
		results.Show()
		assert.Equal(t, 1, len(results))
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
