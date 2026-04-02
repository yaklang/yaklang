<html:form action='/ext/categories/edit_category' styleId="fm">
<input type="hidden" name="cmd" value="<%= Constants.ADD %>">
<html:hidden property="inode"  />
<% if (request.getParameter("parent") != null) { %>
	<input type="hidden" name="redirect" value="<portlet:actionURL>
	<portlet:param name="struts_action" value="/ext/categories/view_category" />
	<portlet:param name="inode" value='<%=request.getParameter("parent")%>' />
	</portlet:actionURL>">
<% } %>
</html:form>
