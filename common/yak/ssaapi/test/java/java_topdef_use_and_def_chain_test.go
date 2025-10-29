package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJava_TopDef_UD_Relationship_Param(t *testing.T) {
	code := `package com.ruoyi.web.controller.common;
import java.io.*;

public class FileUtils {
    public static void writeBytes(String filePath, OutputStream os){
        FileInputStream fis = null;
		File file = new File(filePath);
		fis = new FileInputStream(file);
		byte[] b = new byte[1024];
		int length;
		while ((length = fis.read(b)) > 0) {
			os.write(b, 0, length);
		}
    }
}
@Controller
public class CommonController
{
    @GetMapping("common/download")
    public void fileDownload(String fileName, Boolean delete, HttpServletResponse response, HttpServletRequest request)
	{
            if (!FileUtils.isValidFilename(fileName))
            {
                return;
            }
            String realFileName = System.currentTimeMillis() + fileName.substring(fileName.indexOf("_") + 1);
            String filePath = Global.getDownloadPath() + fileName;
          
            FileUtils.writeBytes(filePath, response.getOutputStream());
    }
}
`
	t.Run("test exclude filter from sink to source", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			sfRule := `
File(,* as $sink);
fileName?{opcode:param} as $source;
$sink #{until: <<<UNTIL
	<self> & $source;
UNTIL,
exclude: <<<EXCLUDE
	<self>?{opcode:call||opcode:phi}
EXCLUDE,
			}-> as $result`
			vals, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)
			res := vals.GetValues("result")
			//res.Show()
			//res.ShowDot()
			require.Nil(t, res)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test exclude filter from sink to source", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			sfRule := `
fileName?{opcode:param} as $source;
FileInputStream(,* as $sink);
$sink #{until: <<<UNTIL
	<self> & $source;
UNTIL,
			exclude: <<<EXCLUDE
		<self>?{opcode:call||opcode:phi}
EXCLUDE,
	}-> as $result`
			vals, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)
			res := vals.GetValues("result")
			require.Nil(t, res)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestJava_TopDef_UD_Relationship_Function(t *testing.T) {
	t.Run("test ud relationship:function -> pointer", func(t *testing.T) {
		code := `package com.example.demo1;
class IFunc {
    public int DoGet(String url);
}

class ImplB implements IFunc {
    @Override
    public int DoGet(String url) {
        return 2;
    }
}

public class Main {
    private IFunc Ifunc;
	private ImplB implb;

    public void main(String[] args) {
        func0(Ifunc.DoGet("123"));
    }
}`

		sfRule := `func0(* #-> * as $param)`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			vals, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)
			res := vals.GetValues("param")
			require.NotNil(t, res)
			path := res.DotGraph()
			require.Contains(t, path, "DoGet")
			require.Contains(t, path, "DoGet")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
