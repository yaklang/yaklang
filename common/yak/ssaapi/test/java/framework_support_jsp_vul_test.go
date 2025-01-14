package java

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestJSP_Vul(t *testing.T) {
	t.Run("Case01-InjectionDirectlyInToDomXssSinkEval.jsp", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("InjectionDirectlyInToDomXssSinkEval.jsp", `
	<%@ page language="java" contentType="text/html; charset=ISO-8859-1"
    pageEncoding="ISO-8859-1"%>
<%@ page import="com.sectooladdict.encoders.HtmlEncoder" %>
<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1">
<title>JavaScript Injection in DOM XSS Sink eval()</title>
</head>
<body>

<%
if (request.getParameter("userinput") == null) {
%>
	Enter your input:<br><br>
	<form name="frmInput" id="frmInput" action="Case36-InjectionDirectlyInToDomXssSinkEval.jsp" method="GET">
		<input type="text" name="userinput" id="userinput"><br>
		<input type=submit value="submit">
	</form>
<%
} 
else {
    try {
	  	    String userinput = request.getParameter("userinput");
			userinput = HtmlEncoder.htmlEncodeAngleBracketsAndQuotes(userinput);
	  	    out.println("<script>\neval(\"" + userinput + "\");</script>");
	  	    out.flush();
    } catch (Exception e) {
        out.println("Exception details: " + e);
    }
} //end of if/else block
%>
</body>
</html>`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()

			rule := `
			out?{<typeName>?{have:'javax.servlet'}}.print*(* as $out);
			check $out;
			$out?{have:'eval' &&  have:'script'} as $sink;
			
			request.getParameter() as $source;
			$sink #{
			until:<<<UNTIL
		<self> & $source
UNTIL
				}-> as $high;
			`
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			val := res.GetValues("high")
			require.Contains(t, val.String(), `ParameterMember-parameter[1].getParameter(Parameter-request,"userinput")`)
			return nil
		})
	})

	t.Run("Case02-ScriptlessInjectionInFormTagActionAttribute.jsp", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("ScriptlessInjectionInFormTagActionAttribute.jsp", `
<%@ page language="java" contentType="text/html; charset=ISO-8859-1"
    pageEncoding="ISO-8859-1"%>
<%@ page import="com.sectooladdict.encoders.HtmlEncoder" %>
<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=ISO-8859-1">
<title>Scriptless Injection in HTML Form Tag Action Attribute scope of the HTML page.</title>
</head>
<body>
<!--
	Contributed by the IronWASP project (http://ironwasp.org/).
	Original Author: Lavakumar Kuppan (lava@ironwasp.org).
-->

<%
if (request.getParameter("userinput") == null) {
%>
	Enter your input:<br><br>
	<form name="frmInput" id="frmInput" method="GET">
		<input type="text" name="userinput" id="userinput"><br>
		<input type=submit value="submit">
	</form>
<%
} 
else {
    try {
	  	    String userinput = request.getParameter("userinput");
			userinput = HtmlEncoder.htmlEncodeAngleBracketsAndQuotes(userinput);
			userinput = userinput.replace(":","");
	  	    out.println("<form action=\"" + userinput + "\">\n"
     			+ "Enter Password: <input name='password' id='password' type='password' value=''/>\n"
     			+ "</form> ");
	  	    out.flush();
    } catch (Exception e) {
        out.println("Exception details: " + e);
    }
} //end of if/else block
%>

</body>
</html>`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()

			rule := `
			out?{<typeName>?{have:'javax.servlet'}}.print*(* as $out);
			check $out;
			$out?{opcode:add}?{have:'<' && have:'='} as $sink;
			request.getParameter() as $source;
			$sink #{
			until:<<<UNTIL
		<self> & $source
UNTIL
}-> as $high;
			`

			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			res.GetAllValuesChain().Show()

			val := res.GetValues("high")
			require.Contains(t, val.String(), `ParameterMember-parameter[1].getParameter(Parameter-request,"userinput")`)
			return nil
		})
	})

}
