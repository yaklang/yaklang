<%@ Page Language="C#" %>
<html>
<body>
<%
    string name = "world";
%>
<h1>Hello <%= name %></h1>
<script runat="server">
    void Page_Load(object sender, EventArgs e) {
        Response.Write("ok");
    }
</script>
</body>
</html>
