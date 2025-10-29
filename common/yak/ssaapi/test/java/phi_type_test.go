package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Phi_Type(t *testing.T) {
	t.Run("test phi type merge", func(t *testing.T) {
		code := `
package com.ruoyi.file.utils;

import java.io.File;
import java.io.IOException;
import java.nio.file.Paths;
import java.util.Objects;
import org.apache.commons.io.FilenameUtils;
import org.springframework.web.multipart.MultipartFile;

class Main{
	public static final String upload(String baseDir, MultipartFile file, String[] allowedExtension)
            throws FileSizeLimitExceededException, IOException, FileNameLengthLimitExceededException,
            InvalidExtensionException
    {
        int fileNamelength = Objects.requireNonNull(file.getOriginalFilename()).length();
        if (fileNamelength > FileUploadUtils.DEFAULT_FILE_NAME_LENGTH)
        {
            throw new FileNameLengthLimitExceededException(FileUploadUtils.DEFAULT_FILE_NAME_LENGTH);
        }

        assertAllowed(file, allowedExtension);

        String fileName = extractFilename(file);

        String absPath = getAbsoluteFile(baseDir, fileName).getAbsolutePath();
        file.transferTo(Paths.get(absPath));
        return getPathFileName(fileName);
    }

}
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `.transferTo?{<getObject><typeName>?{have: MultipartFile}} as $sinkCall;`
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			vals := res.GetValues("sinkCall")
			require.Equal(t, 1, vals.Len())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
