package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Frame_Support_Spring(t *testing.T) {
	t.Skip("TODO: support spring framework")
	t.Run("test  freemarker", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("application.properties", `
	# application.properties
spring.freemarker.enabled=true
spring.freemarker.suffix=.QQQQQ
spring.freemarker.charset=UTF-8
spring.freemarker.content-type=text/html
spring.freemarker.check-template-location=true
spring.freemarker.cache=false
`)
		vf.AddFile("controller.java", `import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;

@Controller
public class GreetingController {

    @GetMapping("/greeting")
    public String greeting(Model model) {
        model.addAttribute("name", "World");
        return "greeting"; 
    }
}
`)
		vf.AddFile("greeting.QQQQQ", `
<!DOCTYPE html>
<html>
<head>
    <title>Greeting</title>
</head>
<body>
    <h1>Hello, ${name}!</h1>
</body>
</html>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()

			rule := `
print() as $print 
$print?{<typeName>?{have:'javax.servlet.http.HttpServletResponse'}} as $sink;
`
			vals, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
			require.NoError(t, err)
			res := vals.GetValues("sink")
			require.NotNil(t, res)
			res.Show()
			return nil
		})
	})
}
