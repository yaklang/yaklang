package test

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/java/template2java"
	"path/filepath"
	"testing"
)

func checkJavaFront(t *testing.T, code string) {
	_, err := java2ssa.Frontend(code, false)
	require.NoError(t, err)
}

func TestCreateJavaTemplate(t *testing.T) {
	tests := []struct {
		fileName  string
		filePath  string
		modelName string
		wants     []string
	}{
		{"demo.jsp", "D:\\java_project\\jspDemo\\src\\main\\webapp\\WEB-INF\\jsp", "demo", []string{
			"package D_.java_project.jspDemo.src.main.webapp.WEB_INF.jsp;",
			"public class demo_jsp",
			`out.write("<html>");`,
			`public void _JavaTemplateService(HttpServletRequest request, HttpServletResponse response)`,
			`var1 = request.getAttribute("var1")`,
		}},
		{"++dmo.jsp.", "../com/org", "demo", []string{
			"package com.org;",
			"public class dmo_jsp_",
		}},
		{"qqq.ftl", "/com.org/A", "demo", []string{
			"public class qqq_ftl",
			"package com.org.A;",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			jt, err := template2java.CreateJavaTemplate(filepath.Join(tt.filePath, tt.fileName))
			require.NoError(t, err)
			require.NotNil(t, jt)
			jt.WritePureText("<html>")
			jt.WriteOutput("var1")
			jt.WriteEscapeOutput("var2")
			jt.Finish()
			fmt.Println(jt.String())
			checkJavaFront(t, jt.String())
			for _, want := range tt.wants {
				require.Contains(t, jt.String(), want)
			}
		})
	}
}
