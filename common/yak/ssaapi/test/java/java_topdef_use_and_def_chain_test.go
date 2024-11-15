package java

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestJava_TopDef_Value_RelationShip(t *testing.T) {
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
		}, ssaapi.WithLanguage(consts.JAVA))
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
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}

func Test_asdpoik(t *testing.T) {
	code := `import javax.servlet.ServletException;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.OutputStream;

public class SecureServlet extends HttpServlet {

    private static final String BASE_DIR = "/usr/local/apache-tomcat/webapps/ROOT/safe_directory/";

    @Override
    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        String requestedFile = request.getParameter("file");

        String path= Util.Check(requestedFile);

        File file = new File(BASE_DIR + path);
        if (!file.getCanonicalPath().startsWith(new File(BASE_DIR).getCanonicalPath())) {
            response.sendError(HttpServletResponse.SC_FORBIDDEN, "Access denied");
            return;
        }
        if (!file.exists()) {
            response.sendError(HttpServletResponse.SC_NOT_FOUND, "File not found");
            return;
        }
        response.setContentType("text/plain");
        try (OutputStream out = response.getOutputStream();
             FileInputStream in = new FileInputStream(file)) {
            byte[] buffer = new byte[4096];
            int length;
            while ((length = in.read(buffer)) > 0) {
                out.write(buffer, 0, length);
            }
        }
    }
}`

	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		sfRule := `
		<include('java-spring-param')> as $source;
<include('java-servlet-param')> as $source;
<include('java-write-filename-sink')> as  $sink;
<include('java-read-filename-sink')> as  $sink;


$sink #{
    include:<<<INCLUDE
<self> & $source;
INCLUDE,
    exclude:<<<EXCLUDE
<self>?{opcode:call}?{!<self> & $source}?{!<self> & $sink};
EXCLUDE
}->as $high;

alert $high for {
    message: "Find direct path travel vulnerability for java",
    type: vuln,
    level: high,
};
		`

		vals, err := prog.SyntaxFlowWithError(sfRule)
		require.NoError(t, err)
		res := vals.GetValues("high")

		res.ShowDot()
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
