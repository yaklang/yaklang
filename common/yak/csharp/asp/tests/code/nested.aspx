<%@ Page Language="C#" %>
<html>
<body>
<div>
  <span>
    <% int n = 1; %>
    <b><%= n %></b>
    <ul>
      <li><%# Eval("A") %></li>
    </ul>
  </span>
</div>
<script runat="server">
void Page_Load(object sender, EventArgs e) {
    int x = 1;
}
</script>
<style type="text/css">body { color: #111; }</style>
</body>
</html>
