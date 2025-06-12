package syntaxflow

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestVirtualImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("security.java", `package com.example.pathtraveldemo.controller;

import java.io.IOException;


@RestController
@RequestMapping("/secure")
public class SecureController {
    @GetMapping("/file")
    public ResponseEntity<?> downloadFileSecure( ){

        try {

        } catch (IOException e) {
            return ResponseEntity.internalServerError().body("文件读取失败: " + e.getMessage());
        }
    }

}   `)
	vf.AddFile("vuln.java", `package com.example.pathtraveldemo.controller;



import java.io.File;
import java.io.IOException;


@RestController
@RequestMapping("/vulnerable")
public class VulnerableController {

    @GetMapping("/file")
    public ResponseEntity<?> downloadFileVulnerable(@RequestParam("filename") String filename) {
        try {
            File file = new File(filename);
        } catch (IOException e) {
            throw new RuntimeException(e);
        }
    }

} 
`)

	ssatest.CheckWithFS(vf, t, func(prog ssaapi.Programs) error {
		// get File's constructor
		result, err := prog.SyntaxFlowWithError("File()<getCallee()> as $value")
		require.NoError(t, err)
		values := result.GetValues("value")
		values.Show()
		fmt.Println(values.DotGraph())
		require.Equal(t, 1, values.Len())

		value := values[0]
		r := value.GetRange()
		require.Equal(t, "vuln.java", r.GetEditor().GetFilename())
		require.NotEqual(t, "", r.GetText())
		return nil
	})
}
