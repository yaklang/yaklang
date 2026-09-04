<%@ Page Language="C#" %>
<%! int Declared = 1; %>
<% int keptScriptlet = 2; %>
<%= keptScriptlet %>
<%# Eval("Name") %>
<html>
<body>
<div class="wrap" id="main" selected>
  <span>
    <% int n = 3; %>
    <b><%= n %></b>
    <a href=<%= n %>>link</a>
    </orphan>
    <ul>
      <li><%# Eval("A") %></li>
    </ul>
  </span>
  <img src="x.png" />
  <br>
  <hr/>
  <input type="text" disabled>
  <td colspan=2>cell</td>
</div>
<script runat="server">
void Page_Load(object sender, EventArgs e) {
    int x = 1;
}
</script>
<style type="text/css">body { color: #111; }</style>
plain text
</body>
</html>
</stray>
