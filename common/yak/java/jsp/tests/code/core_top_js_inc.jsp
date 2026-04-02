<%@page import="com.dotmarketing.cms.factories.PublicCompanyFactory"%>
<%@page import="com.liferay.portal.util.WebKeys"%>
<%@page import="java.util.Locale"%>
<%
	Locale locale = PublicCompanyFactory.getDefaultCompany().getLocale();
%>

	var n1Portlets = new Array();
	var CTX_PATH = '<%= application.getAttribute(WebKeys.CTX_PATH)%>';

	<%
	boolean inFrame = false;
	%>

	function submitFormAlert() {
		alert("test");
	}

	<%
	String[] calendarDays = CalendarUtil.getDays(locale, "EEEE");
	%>

	Calendar._DN = new Array(
		"<%= calendarDays[0] %>",
		"<%= calendarDays[1] %>"
	);
