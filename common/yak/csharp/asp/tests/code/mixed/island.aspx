<%@ Page Language="C#" %>
<script runat="server">
public static int IslandValue() { return 42; }
</script>
<html>
<body>
<% int keptScriptlet = 7; %>
<p><%= IslandValue() %></p>
</body>
</html>
