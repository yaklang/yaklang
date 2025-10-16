package yakgrpc

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SSARisk_Export_And_Import(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	program := uuid.NewString()
	suite, clean := ssatest.NewSFScanRiskTestSuite(t, client, program, consts.JAVA)
	defer clean()

	// 创建虚拟文件系统并添加测试文件
	vf := filesys.NewVirtualFs()
	vf.AddFile("sqli.java", `package com.mycompany.myapp;

import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.util.List;

@Mapper
public interface UserMapper {

    User getUser(@Param("id") Long id);

    void insertUser(User user);

    void updateUser(User user);

    void deleteUser(@Param("id") Long id);

    List<User> getAllUsers();
}`)

	vf.AddFile("sqlmap.xml", `<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE mapper
        PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN"
        "http://mybatis.org/dtd/mybatis-3-mapper.dtd">

<mapper namespace="com.mycompany.myapp.UserMapper">
    <resultMap id="UserResult" type="com.mycompany.myapp.User">
        <id property="id" column="id" />
        <result property="name" column="name" />
        <result property="email" column="email" />
    </resultMap>

    <select id="getUser" resultMap="UserResult">
        SELECT * FROM User WHERE id = #{id}
    </select>

    <insert id="insertUser" useGeneratedKeys="true" keyProperty="id">
        INSERT INTO User (name, email) VALUES (#{name}, #{email})
    </insert>

    <update id="updateUser">
        UPDATE User SET name=#{name}, email=#{email} WHERE id=${id}
    </update>

    <delete id="deleteUser">
        DELETE FROM User WHERE id=#{id}
    </delete>
</mapper>`)

	// 定义扫描规则
	rule := `
	<mybatisSink> as $sink
	alert $sink for {
		title_zh: "MyBatis SQL 注入漏洞",
		type: audit,
		severity: medium,
		desc: "MyBatis SQL 注入漏洞",
	};
	`

	// 使用 suite 进行扫描
	var riskCount int
	var allRisks []*schema.SSARisk
	err = suite.InitProgram(vf).ScanWithRule(rule).HandleLastTaskRisks(func(risks []*schema.SSARisk) error {
		riskCount = len(risks)
		allRisks = risks
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, riskCount, 0, "No risks found after scan")
	t.Logf("Created program '%s' with %d risks", program, riskCount)

	db := ssadb.GetDB()
	targetDir := t.TempDir()
	ctx := utils.TimeoutContextSeconds(30)
	var exportedFile string
	var report sfreport.Report

	// ================ 测试导出 ========================
	t.Run("export risks with dataflow and file content", func(t *testing.T) {
		// 导出风险
		exportStream, err := client.ExportSSARisk(ctx, &ypb.ExportSSARiskRequest{
			Filter: &ypb.SSARisksFilter{
				ProgramName: []string{program},
			},
			TargetPath:       targetDir,
			WithDataFlowPath: true,
			WithFileContent:  true,
		})
		require.NoError(t, err)

		exportProgress := 0.0
		for {
			msg, err := exportStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Logf("export stream error: %v", err)
				}
				break
			}
			t.Logf("Export: %s (progress: %.2f)", msg.Verbose, msg.Process)
			exportProgress = msg.Process
		}
		require.Equal(t, 1.0, exportProgress, "Export did not complete")

		// 验证导出的报告
		files, err := filepath.Glob(filepath.Join(targetDir, "ssa_risk_export_*.json"))
		require.NoError(t, err)
		require.Len(t, files, 1, "Expected one export file")
		exportedFile = files[0]

		exportData, err := os.ReadFile(exportedFile)
		require.NoError(t, err)

		err = json.Unmarshal(exportData, &report)
		require.NoError(t, err)
		require.Equal(t, program, report.ProgramName)
		require.Equal(t, riskCount, len(report.Risks))

		// 检查风险详情
		risks := lo.MapToSlice(report.Risks, func(key string, value *sfreport.Risk) *sfreport.Risk {
			return value
		})
		require.Greater(t, len(risks), 0)
		risk := risks[0]
		require.Equal(t, string(consts.JAVA), risk.GetLanguage())
		require.Equal(t, "MyBatis SQL 注入漏洞", risk.GetTitleVerbose())
		require.Greater(t, len(risk.DataFlowPaths), 0, "Expected data flow paths")

		// 检查文件
		require.Equal(t, 2, len(report.File))
		hasSqliJava := false
		hasSqlmapXml := false
		for _, file := range report.File {
			if strings.Contains(file.Path, "sqli.java") {
				hasSqliJava = true
				require.NotEmpty(t, file.Content, "Expected file content")
			}
			if strings.Contains(file.Path, "sqlmap.xml") {
				hasSqlmapXml = true
				require.NotEmpty(t, file.Content, "Expected file content")
			}
		}
		require.True(t, hasSqliJava)
		require.True(t, hasSqlmapXml)

		t.Logf("Successfully exported %d risks and %d files", riskCount, len(report.File))
	})

	// ================ 测试导入 ========================
	t.Run("import risks with dataflow and file content", func(t *testing.T) {
		// 删除风险和文件以测试导入
		beforeDeleteCount, err := yakit.QuerySSARiskCount(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
		require.NoError(t, err)
		beforeDeleteFileCount, err := ssadb.GetEditorCountByProgramName(program)
		require.NoError(t, err)

		// 删除风险
		err = yakit.DeleteSSARisks(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
		require.NoError(t, err)
		afterDeleteCount, err := yakit.QuerySSARiskCount(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
		require.NoError(t, err)
		require.Equal(t, 0, afterDeleteCount, "Risks not deleted properly")

		// 删除程序（包括文件）
		ssadb.DeleteProgram(db, program)
		afterDeleteFileCount, err := ssadb.GetEditorCountByProgramName(program)
		require.NoError(t, err)
		require.Equal(t, 0, afterDeleteFileCount, "Files not deleted properly")

		t.Logf("Deleted %d risks and %d files", beforeDeleteCount, beforeDeleteFileCount)

		// 导入风险
		importStream, err := client.ImportSSARisk(ctx, &ypb.ImportSSARiskRequest{
			InputPath: exportedFile,
		})
		require.NoError(t, err)

		importProgress := 0.0
		for {
			msg, err := importStream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Logf("import stream error: %v", err)
				}
				break
			}
			t.Logf("Import: %s (progress: %.2f)", msg.Verbose, msg.Process)
			importProgress = msg.Process
		}
		require.Equal(t, 1.0, importProgress, "Import did not complete")

		// 验证导入后的风险
		afterImportCount, err := yakit.QuerySSARiskCount(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
		require.NoError(t, err)
		require.Equal(t, riskCount, afterImportCount, "Risk count mismatch after import")

		_, importedRisks, err := yakit.QuerySSARisk(db, &ypb.SSARisksFilter{
			ProgramName: []string{program},
		}, nil)
		require.NoError(t, err)
		require.Equal(t, riskCount, len(importedRisks))

		// 验证风险标题匹配
		riskTitleMap := make(map[string]bool)
		for _, risk := range importedRisks {
			riskTitleMap[risk.Title] = true
		}
		for _, risk := range allRisks {
			require.True(t, riskTitleMap[risk.Title], "Missing risk title: %s", risk.Title)
		}

		// 验证导入后的文件
		afterImportFileCount, err := ssadb.GetEditorCountByProgramName(program)
		require.NoError(t, err)
		require.Equal(t, len(report.File), afterImportFileCount, "File count mismatch after import")

		importedFiles, err := ssadb.GetEditorByProgramName(program)
		require.NoError(t, err)
		require.Equal(t, len(report.File), len(importedFiles))

		// 创建导出文件的路径映射
		filePathMap := make(map[string]bool)
		for _, file := range report.File {
			filePathMap[file.Path] = true
		}

		// 验证所有导出的文件都被导入了
		for _, importedFile := range importedFiles {
			require.True(t, filePathMap[importedFile.GetUrl()], "Imported file not in exported files: %s", importedFile.GetUrl())
		}

		t.Logf("Successfully imported %d risks and %d files", riskCount, len(report.File))
	})
}
