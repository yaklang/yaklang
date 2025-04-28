package tests

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed code/DynamicSecurityMetadataSource.java
var DynamicSecurityMetadataSource string

func TestRealJava_PanicInMemberCall(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("DynamicSecurityMetadataSource.java", DynamicSecurityMetadataSource)
	ssatest.CheckWithFS(vf, t, func(prog ssaapi.Programs) error {
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestA(t *testing.T) {

	code := `
package org.joychou.controller;



public class ClassDataLoader {
    @Override
    public void doFilter(ServletRequest servletRequest, ServletResponse servletResponse, FilterChain filterChain) throws IOException, ServletException {
        String cmd = servletRequest.getParameter("cmd_");
		Process process = Runtime.getRuntime().exec(cmd);
    }
}
	`

	ssatest.CheckSyntaxFlow(t, code, `

<include('java-servlet-param')> as $source;
<include('java-spring-mvc-param')> as $source;
check $source;

<include('java-runtime-exec-sink')> as $sink;
<include('java-command-exec-sink')> as $sink;
check $sink;

$sink #{
	until:"<self> & $source",
}->as $high;

	`, map[string][]string{}, ssaapi.WithLanguage(consts.JAVA))
}
