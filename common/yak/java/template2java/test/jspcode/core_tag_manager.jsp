<%@page import="com.dotmarketing.util.Config"%>
<%@ page import="com.liferay.portal.language.LanguageUtil"%>
<%@ page import="com.liferay.portal.util.ReleaseInfo"%>

<html xmlns="http://www.w3.org/1999/xhtml">
<%
	String dojoPath = Config.getStringProperty("path.to.dojo");
%>

<style type="text/css">
@import "<%=dojoPath%>/dojox/grid/enhanced/resources/claro/EnhancedGrid.css?b=<%= ReleaseInfo.getVersion() %>";
</style>

<script type="text/javascript">
    var addTagMsg = '<%= LanguageUtil.get(pageContext, "add-tag")%>';
    var editTagMsg = '<%= LanguageUtil.get(pageContext, "edit-tag")%>';
</script>

<%-- Add Tag Dialog --%>
<div id="addTagDialog">
    <span><%= addTagMsg %></span>
</div>
<%-- /Add Tag Dialog --%>

<%-- Import Tag Dialog --%>
<jsp:include page="/html/portlet/ext/browser/sub_nav.jsp"></jsp:include>
<%-- /Import Tag Dialog --%>
</html>
