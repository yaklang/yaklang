package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestDataflowReal1(t *testing.T) {
	t.Run("test file read", func(t *testing.T) {
		code := `
package com.ruoyi.common.utils.file;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.OutputStream;
import java.io.UnsupportedEncodingException;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;


public class FileUtils extends org.apache.commons.io.FileUtils
{
    public static String FILENAME_PATTERN = "[a-zA-Z0-9_\\-\\|\\.\\u4e00-\\u9fa5]+";

    /**
     * 输出指定文件的byte数组
     * 
     * @param filePath 文件路径
     * @param os 输出流
     * @return
     */
    public static void writeBytes(String filePath, OutputStream os) throws IOException
    {
        FileInputStream fis = null;
        try
        {
            File file = new File(filePath);
            if (!file.exists())
            {
                throw new FileNotFoundException(filePath);
            }
            fis = new FileInputStream(file);
            byte[] b = new byte[1024];
            int length;
            while ((length = fis.read(b)) > 0)
            {
                os.write(b, 0, length);
            }
        }
        catch (IOException e)
        {
            throw e;
        }
        finally
        {
            if (os != null)
            {
                try
                {
                    os.close();
                }
                catch (IOException e1)
                {
                    e1.printStackTrace();
                }
            }
            if (fis != null)
            {
                try
                {
                    fis.close();
                }
                catch (IOException e1)
                {
                    e1.printStackTrace();
                }
            }
        }
    }
   
}
	`

		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `
File() as $fileInstance 
$fileInstance -{
	include: <<<CODE
	.read()
CODE
}-> as $fileReadInstance 
		`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			file := vals.GetValues("fileInstance")
			file.Show()
			require.Contains(t, file.String(), `Undefined-File(Undefined-File,Parameter-filePath)`)
			fileRead := vals.GetValues("fileReadInstance")
			fileRead.Show()
			require.Contains(t, fileRead.String(), `Undefined-fis.read`)

			return nil
		}, ssaapi.WithRawLanguage("java"))
	})

	t.Run("test ddos real", func(t *testing.T) {
		code := `
package org.example.Dos;

import java.io.*;
import java.net.Socket;

public class DOSDemo {
    public static void readSocketData(Socket socket) throws IOException {
        BufferedReader reader = new BufferedReader(
                new InputStreamReader(socket.getInputStream())
        );
        String line;
        // 限制单行的最大长度
        final int MAX_LINE_LENGTH = 1024; // 最大行长度为1024个字符
        while ((line = reader.readLine()) != null) {
            processLine(line);
        }
    }
}
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `
.getInputStream()?{<fullTypeName>?{have: 'java.net.Socket' || 'java.new.ServerSocket'}} as $source;
BufferedReader().readLine()?{!.length}?{<fullTypeName>?{have:'java.io'}}  as $sink;
$sink#{
    include:<<<CODE
    <self> & $source
CODE
}-> as $vul;
`
			vals, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug())
			require.NoError(t, err)
			source := vals.GetValues("vul")
			require.Contains(t, source.String(), `Parameter-socket`)
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}

func TestDataflowTest(t *testing.T) {
	code := `
	a = {} 

	source := a.b()
	{
		b = source + 1 
		b = c(b)
		f1(b)
	}
	`

	ssatest.CheckSyntaxFlow(t, code, `
a.b() as $source 
f1(* as $sink)
$sink #-> as $vul1
$sink<dataflow(<<<CODE
    * ?{opcode: const} as $value1
CODE)> 
    `, map[string][]string{
		"value1": {"1"},
	})
}

func TestUntil(t *testing.T) {
	rule := `
a as $source
b(* as $sink)
$sink #{
    until: "* & $source"
}-> as $target 
`

	t.Run("test until", func(t *testing.T) {
		code := `
a = 12344
cc = [1, 2 , a]
b(cc)
    `

		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"target": {"12344"},
		}, ssaapi.WithLanguage(ssaapi.Yak))

	})

	t.Run("test until not match", func(t *testing.T) {
		code := `
a = 12344
cc = [1, 2 , 3]
b(cc)
`
		ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
			"target": {},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
}
