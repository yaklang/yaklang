package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/A.java", `
	package A; 
	class A {
		public  int get() {
			return 	 1;
		}
	}
	`)
	vf.AddFile("src/main/java/B.java", `
	package B; 
	import A.A;
	class B {
		public static void main(String[] args) {
			A a = new A();
			println(a.get());
		}
	}
	`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		println(* #-> as $a)
		`, map[string][]string{
		"a": {"1"},
	}, false, ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestImport_FilePath(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/io/github/talelin/latticy/common/aop/ResultAspect.java", `package io.github.talelin.latticy.common.aop;
import io.github.talelin.latticy.module.log.MDCAccessServletFilter;

@Aspect
@Component
public class ResultAspect {
    public void doAfterReturning(UnifyResponseVO<String> result) {
    }
}
`)

	vf.AddFile(`src/main/java/io/github/talelin/latticy/module/log/MDCAccessServletFilter.java`, `
package io.github.talelin.latticy.module.log;

public class MDCAccessServletFilter implements Filter {
    @Override
    public void doFilter(ServletRequest request, ServletResponse response, FilterChain chain) throws IOException, ServletException {
    }
}
`)
	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		result, err := prog.SyntaxFlowWithError(`doFilter`)
		if err != nil {
			return err
		}
		log.Info(result.String())
		require.Contains(t, result.String(), "src/main/java/io/github/talelin/latticy/module/log/MDCAccessServletFilter.java")
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))

}
