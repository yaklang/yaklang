package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadSpringResponseBodyXSSRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-79-xss/java-spring-response-body-xss.sf")
	if !ok {
		t.Skip("java-spring-response-body-xss.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-spring-response-body-xss.sf 内容为空")
	return content
}

func TestSpringResponseBodyXSSRule_Positive_StringReturn(t *testing.T) {
	rule := loadSpringResponseBodyXSSRule(t)
	code := `
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/xss")
public class XSSController {
    @GetMapping("/echo")
    public String echo(@RequestParam("input") String input) {
        return "hello " + input;
    }
}
`

	counts := runJavaRule(t, rule, "XSSController.java", code)
	assert.Greater(t, counts["withoutCall"], 0, "直接返回用户输入字符串应触发高危告警")
	assert.Zero(t, counts["filteredSink"], "未过滤场景不应落到已过滤审计分支")
}

func TestSpringResponseBodyXSSRule_Negative_MapReturn(t *testing.T) {
	rule := loadSpringResponseBodyXSSRule(t)
	code := `
import java.util.HashMap;
import java.util.Map;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/xss")
public class XSSController {
    @GetMapping("/echo")
    public Map<String, Object> echo(@RequestParam("input") String input) {
        Map<String, Object> modelMap = new HashMap<>();
        modelMap.put("payload", input);
        modelMap.put("status", "ok");
        return modelMap;
    }
}
`

	counts := runJavaRule(t, rule, "XSSController.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "返回 JSON/Map 场景不应由这条字符串响应 XSS 规则报出")
}

func TestSpringResponseBodyXSSRule_Filtered_StringReturn(t *testing.T) {
	rule := loadSpringResponseBodyXSSRule(t)
	code := `
import org.springframework.web.bind.annotation.*;
import org.springframework.web.util.HtmlUtils;

@RestController
@RequestMapping("/xss")
public class XSSController {
    @GetMapping("/echo")
    public String echo(@RequestParam("input") String input) {
        return "hello " + HtmlUtils.htmlEscape(input);
    }
}
`

	counts := runJavaRule(t, rule, "XSSController.java", code)
	assert.Zero(t, counts["withoutCall"], "已过滤场景不应落到未过滤高危分支")
	assert.Greater(t, counts["filteredSink"], 0, "已识别过滤函数时应落到审计分支")
}
