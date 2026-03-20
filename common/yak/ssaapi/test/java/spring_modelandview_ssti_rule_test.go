package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func loadSpringModelAndViewSSTIRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-1336-ssti/java-spring-framework-model-controllable.sf")
	if !ok {
		t.Skip("java-spring-framework-model-controllable.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-spring-framework-model-controllable.sf 内容为空")
	return content
}

func runJavaRule(t *testing.T, ruleContent, filename, code string) map[string]int {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)

	counts := make(map[string]int)
	programs, err := ssaapi.ParseProjectWithFS(vfs, ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err, "SSA 编译不应报错")
	require.Greater(t, len(programs), 0, "SSA 编译应至少产生一个程序")

	result, err := programs[0].SyntaxFlowWithError(ruleContent)
	require.NoError(t, err, "规则执行不应报错")
	for _, varName := range result.GetAlertVariables() {
		counts[varName] = len(result.GetValues(varName))
	}

	return counts
}

func totalAlerts(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func TestSpringModelAndViewSSTIRule_Positive_ConstructorViewName(t *testing.T) {
	rule := loadSpringModelAndViewSSTIRule(t)
	code := `
import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.servlet.ModelAndView;

@Controller
public class OrgConsoleController {
    @GetMapping("/edit")
    public ModelAndView edit(@RequestParam String id) {
        return new ModelAndView("/admin/org" + id + "/edit.html");
    }
}
`

	counts := runJavaRule(t, rule, "OrgConsoleController.java", code)
	assert.Greater(t, counts["filteredSource"], 0, "用户输入控制 ModelAndView 视图名时应触发告警")
}

func TestSpringModelAndViewSSTIRule_Negative_QueryAndAddObjectOnly(t *testing.T) {
	rule := loadSpringModelAndViewSSTIRule(t)
	code := `
import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.servlet.ModelAndView;

@Controller
public class OrgConsoleController {
    @GetMapping("/edit")
    public ModelAndView edit(@RequestParam String id) {
        ModelAndView view = new ModelAndView("/admin/org/edit.html");
        Object org = orgConsoleService.queryById(id);
        view.addObject("org", org);
        return view;
    }
}
`

	counts := runJavaRule(t, rule, "OrgConsoleController.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "参数仅用于查询和 addObject，不应被当成模板路径控制")
}

func TestSpringModelAndViewSSTIRule_Positive_SetViewName(t *testing.T) {
	rule := loadSpringModelAndViewSSTIRule(t)
	code := `
import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.servlet.ModelAndView;

@Controller
public class OrgConsoleController {
    @GetMapping("/edit")
    public ModelAndView edit(@RequestParam String id) {
        ModelAndView view = new ModelAndView();
        view.setViewName("/admin/org" + id + "/edit.html");
        return view;
    }
}
`

	counts := runJavaRule(t, rule, "OrgConsoleController.java", code)
	assert.Greater(t, counts["filteredSource"], 0, "用户输入经 setViewName 控制视图名时应触发告警")
}
