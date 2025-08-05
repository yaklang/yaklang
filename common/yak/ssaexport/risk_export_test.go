package ssaexport

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRiskExport_CompleteFlow(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("FileUploader.java", `import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.StandardCopyOption;

public class FileUploader {
    // 允许上传的文件扩展名白名单
    private static final String[] ALLOWED_EXTENSIONS = {".jpg", ".jpeg", ".png", ".gif", ".pdf", ".txt"};
    // 上传文件存储的基础目录
    private static final String UPLOAD_BASE_DIR = "/var/www/uploads/";

    /**
     * 安全上传文件方法
     * @param inputStream 文件输入流
     * @param fileName 原始文件名
     * @param subDir 子目录（可选）
     * @return 上传后的文件路径
     * @throws IOException 如果上传过程中发生错误
     * @throws SecurityException 如果检测到不安全操作
     */
    public static String uploadFile(InputStream inputStream, String fileName, String subDir)
            throws IOException, SecurityException {

        // 1. 检查文件名是否合法
        if (fileName == null || fileName.isEmpty()) {
            throw new SecurityException("文件名不能为空");
        }

        // 2. 防止路径穿越攻击
        if (fileName.contains("../") || fileName.contains("..\\")) {
            throw new SecurityException("文件名包含非法路径字符");
        }

        // 如果指定了子目录，同样检查子目录是否合法
        if (subDir != null && !subDir.isEmpty()) {
            if (subDir.contains("../") || subDir.contains("..\\")) {
                throw new SecurityException("子目录包含非法路径字符");
            }
        }

        // 3. 检查文件扩展名是否合法
        String fileExtension = getFileExtension(fileName).toLowerCase();
        boolean allowed = false;
        for (String ext : ALLOWED_EXTENSIONS) {
            if (ext.equalsIgnoreCase(fileExtension)) {
                allowed = true;
                break;
            }
        }
        if (!allowed) {
            throw new SecurityException("不允许的文件类型: " + fileExtension);
        }

        // 4. 创建目标目录
        Path uploadDir = Paths.get(UPLOAD_BASE_DIR, subDir != nil ? subDir : "");
        if (!Files.exists(uploadDir)) {
            Files.createDirectories(uploadDir);
        }

        // 5. 生成安全的文件名（避免覆盖现有文件）
        String safeFileName = System.currentTimeMillis() + "_" + fileName;
        Path destination = uploadDir.resolve(safeFileName);

        // 6. 保存文件
        Files.copy(inputStream, destination, StandardCopyOption.REPLACE_EXISTING);

        // 7. 返回相对路径（不暴露服务器绝对路径）
        return Paths.get(subDir != nil ? subDir : "", safeFileName).toString();
    }

    /**
     * 获取文件扩展名
     * @param fileName 文件名
     * @return 文件扩展名（包含点）
     */
    private static String getFileExtension(String fileName) {
        int dotIndex = fileName.lastIndexOf('.');
        if (dotIndex > 0 && dotIndex < fileName.length() - 1) {
            return fileName.substring(dotIndex);
        }
        return "";
    }

    // 示例用法
    public static void main(String[] args) {
        try {
            // 模拟文件上传
            InputStream fileStream = FileUploader.class.getResourceAsStream("/test.txt");
            String uploadedPath = uploadFile(fileStream, "test.txt", "user_docs");
            System.out.println("文件上传成功，路径: " + uploadedPath);
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}`)

	ssatest.CheckProfileWithFS(vf, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		if p != ssatest.OnlyDatabase {
			return nil
		}
		result, err := prog.SyntaxFlowWithError(`
desc(
	title:"this is a audit test rule"
)

Files.copy #-> as $a
alert $a for{
    title:"File Upload Risk"
    title_zh:"文件上传风险"
    level:high
    desc:"检测到文件上传操作，可能存在安全风险"
}`, ssaapi.QueryWithEnableDebug(true))
		require.NoError(t, err)

		resultId, err := result.Save(schema.SFResultKindDebug)
		require.NoError(t, err)
		require.NotZero(t, resultId)

		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{result.GetProgramName()},
		}, nil)
		require.NoError(t, err)
		require.NotEmpty(t, risks, "应该生成风险数据")

		temp := t.TempDir()
		outputPath := filepath.Join(temp, "risk_export_test.json")

		err = ExportSSARisksToJSON(risks, outputPath)
		require.NoError(t, err)

		// 验证输出文件存在
		_, err = os.Stat(outputPath)
		require.NoError(t, err)

		// 读取并验证JSON内容
		jsonData, err := os.ReadFile(outputPath)
		require.NoError(t, err)

		var exportData RiskExportData
		err = json.Unmarshal(jsonData, &exportData)
		require.NoError(t, err)

		// 验证基本结构
		require.NotZero(t, exportData.ExportTime)
		require.Equal(t, len(risks), exportData.TotalRisks)
		require.Len(t, exportData.Risks, len(risks))

		// 验证风险数据
		for i, riskItem := range exportData.Risks {
			require.Equal(t, result.GetProgramName(), riskItem.ProjectInformation.ProgramName)
			require.Equal(t, "java", riskItem.ProjectInformation.Language)
			require.Equal(t, risks[i].Title, riskItem.DetailInformation.Title)
			require.Equal(t, risks[i].RiskType, riskItem.DetailInformation.RiskType)
			require.Equal(t, string(risks[i].Severity), riskItem.DetailInformation.Severity)
		}
		return nil
	})
}
