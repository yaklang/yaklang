package sfreport_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/samber/lo"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReport(t *testing.T) {

	vf := filesys.NewVirtualFs()
	vf.AddFile("a.java", `
package org.joychou.controller;

public class SQLI {
    @RequestMapping("/jdbc/vuln")
    public String jdbc_sqli_vul(@RequestParam("username") String username) {

        StringBuilder result = new StringBuilder();

        try {
            Class.forName(driver);
            Connection con = DriverManager.getConnection(url, user, password);

            if (!con.isClosed())
                System.out.println("Connect to database successfully.");

            // sqli vuln code
            Statement statement = con.createStatement();
            String sql = "select * from users where username = '" + username + "'";
            logger.info(sql);
            ResultSet rs = statement.executeQuery(sql);

            while (rs.next()) {
                String res_name = rs.getString("username");
                String res_pwd = rs.getString("password");
                String info = String.format("%s: %s\n", res_name, res_pwd);
                result.append(info);
                logger.info(info);
            }
            rs.close();
            con.close();


        } catch (ClassNotFoundException e) {
            logger.error("Sorry, can't find the Driver!");
        } catch (SQLException e) {
            logger.error(e.toString());
        }
        return result.toString();
    }

	@RequestMapping("/jdbc/vuln")
    public String jdbc_sqli_vul(@RequestParam("username") String username) {

        StringBuilder result = new StringBuilder();

        try {
            Class.forName(driver);
            Connection con = DriverManager.getConnection(url, user, password);

            if (!con.isClosed())
                System.out.println("Connect to database successfully.");

            // sqli vuln code
            Statement statement = con.createStatement();
            String sql = "select * from users where username = '" + username + "'";
            logger.info(sql);
            ResultSet rs = statement.executeQuery(sql);

            while (rs.next()) {
                String res_name = rs.getString("username");
                String res_pwd = rs.getString("password");
                String info = String.format("%s: %s\n", res_name, res_pwd);
                result.append(info);
                logger.info(info);
            }
            rs.close();
            con.close();


        } catch (ClassNotFoundException e) {
            logger.error("Sorry, can't find the Driver!");
        } catch (SQLException e) {
            logger.error(e.toString());
        }
        return result.toString();
    }


}
	`)

	progName := uuid.NewString()
	prog, err := ssaapi.ParseProject(ssaapi.WithFileSystem(vf), ssaapi.WithLanguage(consts.JAVA), ssaapi.WithProgramName(progName))
	require.NoError(t, err)

	rule := `
g"SELECT*" as $sqlConst;
g"select*" as $sqlConst;

// 检测 SQL 字符串被传入到了某一个执行函数中，执行函数符合常见的 SQL 执行命名规范
$sqlConst -{
	until: <<<CODE
*?{opcode: call && <getCallee><name>?{have: /(?i)(query)|(execut)|(insert)|(native)|(update)/}<show>}<var(sink)> as $__next__;
CODE
}->;
check $sink;

// 检测 SQL 字符串是否被 add 操作拼接，add 操作是字符串拼接的常见操作
// 这里虽然会不全面，但是可以作为一个案例，可以支持更多规则来实现这个细节检测
$sqlConst?{<self>#>?{opcode: add}<var(op)> || <self>->?{opcode: add}<var(op)>};
check $op;

alert $op for {
	title_zh: "SQL 字符串拼接位置：疑似 SQL 语句拼接并执行到数据库查询的代码",
	type: audit,
	severity: medium,
	desc: "疑似 SQL 语句拼接并执行到数据库查询的代码",
};
`
	res, err := prog.SyntaxFlowWithError(rule)
	require.NoError(t, err)

	id, err := res.Save(schema.SFResultKindDebug)
	require.NoError(t, err)
	_ = id

	/*
		{
		  "report_type": "irify",
		  "engine_version": "dev",
		  "report_time": "2025-04-17T15:41:44.631769+08:00",
		  "program_name": "85c90742-d190-4434-a74e-db8dd561b6dd",
		  "Rules": [
		    {
		      "rule_name": "",
		      "language": "",
		      "description": "",
		      "solution": "",
		      "content": "\ng\"SELEC....",
		      "risks": [
		        "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3",
		        "f0481807c1b3d08d9e56fa15b76170bfbcb10898"
		      ]
		    }
		  ],
		  "Risks": {
		    "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3": {
		      "id": 1710,
		      "hash": "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3",
		      "title": "",
		      "title_verbose": "SQL 字符串拼接位置：疑似 SQL 语句拼接并执行到数据库查询的代码",
		      "description": "",
		      "solution": "",
		      "severity": "middle",
		      "risk_type": "其他",
		      "details": "",
		      "cve": "",
		      "time": "2025-04-17T15:41:44.628129+08:00",
		      "code_source_url": "a.java",
		      "line": 19,
		      "code_range": "{\"url\":\"/85c90742-d190-4434-a74e-db8dd561b6dd/a.java\",\"start_line\":19,\"start_column\":26,\"end_line\":19,\"end_column\":77,\"source_code_line\":15}",
		      "rule_name": "",
		      "program_name": "85c90742-d190-4434-a74e-db8dd561b6dd"
		    },
		    "f0481807c1b3d08d9e56fa15b76170bfbcb10898": {
		      "id": 1711,
		      "hash": "f0481807c1b3d08d9e56fa15b76170bfbcb10898",
		      "title": "",
		      "title_verbose": "SQL 字符串拼接位置：疑似 SQL 语句拼接并执行到数据库查询的代码",
		      "description": "",
		      "solution": "",
		      "severity": "middle",
		      "risk_type": "其他",
		      "details": "",
		      "cve": "",
		      "time": "2025-04-17T15:41:44.62919+08:00",
		      "code_source_url": "a.java",
		      "line": 56,
		      "code_range": "{\"url\":\"/85c90742-d190-4434-a74e-db8dd561b6dd/a.java\",\"start_line\":56,\"start_column\":26,\"end_line\":56,\"end_column\":77,\"source_code_line\":52}",
		      "rule_name": "",
		      "program_name": "85c90742-d190-4434-a74e-db8dd561b6dd"
		    }
		  },
		  "File": [
		    {
		      "path": "a.java",
		      "length": 2561,
		      "hash": {
		        "md5": "f239e50e36e4b402df4119d4b6aabe86",
		        "sha1": "f9010aa419c7c094722218ca1cee2332169df3bb",
		        "sha256": "2882ba1aff1e56775a199973613b556d6d3b2ea22c06cb18550e75f6c8577b3b"
		      },
		      "content": "\npackage org.joychou.controller;\n\npublic class SQLI {\n    @RequestMapping(\"/jdbc/vuln\")\n    public S...",
		      "risks": [
		        "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3",
		        "f0481807c1b3d08d9e56fa15b76170bfbcb10898"
		      ]
		    }
		  ]
		}
	*/
	report := sfreport.NewReport(sfreport.IRifyReportType)
	report.AddSyntaxFlowResult(res)

	err = report.PrettyWrite(os.Stdout)
	require.NoError(t, err)

	// check report
	require.Len(t, report.Risks, 2)
	// check report.risk
	for hash, risk := range report.Risks {
		riskDB, err := yakit.GetSSARiskByHash(ssadb.GetDB(), hash)
		require.NoError(t, err)
		require.Equal(t, risk.GetProgramName(), riskDB.ProgramName)
		require.Equal(t, risk.GetHash(), riskDB.Hash)
		require.Equal(t, risk.GetTitle(), riskDB.Title)
		require.Equal(t, risk.GetTitleVerbose(), riskDB.TitleVerbose)
		require.Equal(t, risk.GetDescription(), riskDB.Description)
		require.Equal(t, risk.GetSolution(), riskDB.Solution)
		require.Equal(t, risk.GetSeverity(), string(riskDB.Severity))
		require.Equal(t, risk.GetRiskType(), riskDB.RiskType)
	}

	// check report.rule
	require.Equal(t, len(report.Rules), 1)
	require.Equal(t, report.Rules[0].Content, rule)

	// check report.file
	require.Equal(t, len(report.File), 1)
	require.Equal(t, report.File[0].Path, "a.java")

	// check report.program
	require.Equal(t, report.ProgramName, progName)

}

func TestRiskImportAndExportWithDataFlow(t *testing.T) {
	program := uuid.NewString()
	suite, clean := ssatest.NewSFScanRiskTestSuite(t, program, consts.JAVA)
	defer clean()
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

    List<User> getAllUsers(); // 可选，获取所有用户
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

	rule := `
	<mybatisSink> as $sink
	alert $sink for {
		title_zh: "MyBatis SQL 注入漏洞",
		type: audit,
		severity: medium,
		desc: "MyBatis SQL 注入漏洞",
	};
	`
	reporter := sfreport.NewReport(
		sfreport.IRifyReportType,
		sfreport.WithDataflowPath(true),
		sfreport.WithFileContent(true),
	)

	riskCount := 0
	allRisks := make([]*schema.SSARisk, 0)

	err := suite.InitProgram(vf).ScanWithRule(rule).HandleLastTaskRisks(func(risks []*schema.SSARisk) error {
		reporter.AddSyntaxFlowRisks(risks...)
		riskCount += len(risks)
		allRisks = append(allRisks, risks...)
		return nil
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	reporter.PrettyWrite(&buf)
	t.Run("test export report", func(t *testing.T) {
		require.NotEmpty(t, buf.String())
		t.Logf("report: %s", buf.String())

		var checkReport sfreport.Report
		err := json.Unmarshal(buf.Bytes(), &checkReport)
		require.NoError(t, err)
		require.Equal(t, program, checkReport.ProgramName)
		require.Equal(t, riskCount, len(checkReport.Risks))
		// check risk
		risks := lo.MapToSlice(checkReport.Risks, func(key string, value *sfreport.Risk) *sfreport.Risk {
			return value
		})
		risk := risks[0]
		require.Equal(t, string(consts.JAVA), risk.GetLanguage())
		require.Greater(t, len(risk.DataFlowPaths), 0)
		require.Equal(t, "MyBatis SQL 注入漏洞", risk.GetTitleVerbose())

		// check file
		files := checkReport.File
		require.Equal(t, len(files), 2)
		hasSqliJava := false
		hasSqlmapXml := false
		for _, file := range files {
			if strings.Contains(file.Path, "sqli.java") {
				hasSqliJava = true
			}
			if strings.Contains(file.Path, "sqlmap.xml") {
				hasSqlmapXml = true
			}
		}
		require.True(t, hasSqliJava)
		require.True(t, hasSqlmapXml)
	})

	t.Run("test import report to database", func(t *testing.T) {
		callBack := func(msg string, progress float64) {
			t.Logf("import report to database: %s, progress: %f", msg, progress)
		}
		// 删除后导入才不会重复
		clean()
		require.NoError(t, err)
		files := reporter.File

		db := ssadb.GetDB()
		// ================ 查询导入前的风险和文件 ========================
		beforeImportRiskCount, err := yakit.QuerySSARiskCount(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
		require.NoError(t, err)
		// 查询导入前的文件
		beforeImportFileCount, err := ssadb.GetEditorCountByProgramName(program)
		require.NoError(t, err)

		// ================ 导入risk ========================
		err = sfreport.ImportSSARiskFromJSON(context.Background(), db, buf.Bytes(), callBack)
		require.NoError(t, err)
		afterImportRiskCount, err := yakit.QuerySSARiskCount(db, &ypb.SSARisksFilter{ProgramName: []string{program}})
		require.NoError(t, err)
		require.Equal(t, beforeImportRiskCount+riskCount, afterImportRiskCount)
		// ================ 查询导入后的风险 ========================
		_, importedRisks, err := yakit.QuerySSARisk(db, &ypb.SSARisksFilter{
			ProgramName: []string{program},
		}, nil)
		require.NoError(t, err)
		require.Equal(t, riskCount, len(importedRisks))
		riskTitleMap := make(map[string]bool)
		for _, risk := range importedRisks {
			riskTitleMap[risk.Title] = true
		}
		for _, risk := range allRisks {
			require.True(t, riskTitleMap[risk.Title])
		}
		// ================ 查询导入后的文件 ========================
		afterImportFileCount, err := ssadb.GetEditorCountByProgramName(program)
		require.NoError(t, err)
		require.Equal(t, beforeImportFileCount+len(files), afterImportFileCount)
		filePathMap := make(map[string]bool)
		for _, file := range files {
			filePathMap[file.Path] = true
		}

		queryFiles, err := ssadb.GetEditorByProgramName(program)
		require.NoError(t, err)
		for _, queryFile := range queryFiles {
			require.True(t, filePathMap[queryFile.GetUrl()])
		}
	})
}
