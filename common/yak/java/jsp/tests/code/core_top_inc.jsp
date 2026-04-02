<%@page import="com.liferay.portal.util.WebKeys"%>
<%@page import="com.dotmarketing.util.Config"%>
<%@page import="com.dotmarketing.util.ConfigUtils"%>
<%@page import="com.dotmarketing.util.UtilMethods"%>
<%
	String dojoPath = Config.getStringProperty("path.to.dojo");
	String dojoLocaleConfig = "locale:'en-us'";
%>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">

<html xmlns="http://www.w3.org/1999/xhtml" xmlns:bi="urn:bi" xmlns:csp="urn:csp">
<head>
	<script src="/html/js/dragula-3.7.2/dragula.min.js"></script>
	<link rel="stylesheet" type="text/css" href="<%=dojoPath%>/dijit/themes/dijit.css">
	<%
		if (ConfigUtils.isFeatureFlagOn("FEATURE_FLAG_NEW_BINARY_FIELD")) {
	%>
		<link rel="stylesheet" href="/dotcms-binary-field-builder/styles.css" />
	<% } %>
	<script type="text/javascript">
		djConfig={
			parseOnLoad: true,
			i18n: "<%=dojoPath%>/custom-build/build/",
			useXDomain: false,
			isDebug: false,
			<%=dojoLocaleConfig%>
			modulePaths: {
				dotcms: "/html/js/dotcms",
				vs: "/html/assets/monaco-editor/min/vs"
			}
		};
	</script>
</head>
<body class="dotcms" style="visibility:hidden;background:white">
</body>
</html>
