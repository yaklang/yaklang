<%@ Page Language="C#" %>
<form method="post">
<input type="text" name="q" />
<% if (Request["q"] != null) { %>
<p>query: <%= Request["q"] %></p>
<% } %>
</form>
