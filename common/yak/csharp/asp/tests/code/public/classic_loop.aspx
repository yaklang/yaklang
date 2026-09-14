<%@ Page Language="C#" %>
<!-- Typical classic ASP/ASPX mixed markup loop (scriptlet around HTML). -->
<html>
<body>
<table>
<% for (int i = 0; i < 3; i++) { %>
  <tr>
    <td><%= i %></td>
    <td><%# Eval("Row") %></td>
  </tr>
<% } %>
</table>
</body>
</html>
