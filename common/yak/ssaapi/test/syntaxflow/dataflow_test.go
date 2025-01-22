package syntaxflow

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"testing"

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
			require.Contains(t, file.String(), `Undefined-File(Undefined-File,Parameter-filePath)`)
			fileRead := vals.GetValues("fileReadInstance")
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
.getInputStream()?{<typeName>?{have: 'java.net.Socket' || 'java.new.ServerSocket'}} as $source;
BufferedReader().readLine()?{!.length}?{<typeName>?{have:'java.io'}}  as $sink;
$sink#{
    include:<<<CODE
    <self> & $source
CODE
}-> as $vul;
`
			vals, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			source := vals.GetValues("vul")
			require.Contains(t, source.String(), `Parameter-socket`)
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}
