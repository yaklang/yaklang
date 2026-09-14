<%@ Page Language="C#" AutoEventWireup="true" %>
<!-- Typical ASP.NET Web Forms page (MSDN / Web Forms pattern): server form, label, button, repeater. -->
<html>
<head>
<title>WebForms</title>
<style type="text/css">.row { color: #333; }</style>
</head>
<body>
<form runat="server" method="post">
  <asp:Label ID="lbl" runat="server" Text="hello" />
  <asp:TextBox ID="q" runat="server" />
  <asp:Button ID="go" runat="server" Text="Go" OnClick="Go_Click" />
  <asp:Repeater ID="list" runat="server">
    <ItemTemplate>
      <span class="row"><%# Eval("Name") %></span>
    </ItemTemplate>
  </asp:Repeater>
</form>
<script runat="server">
void Go_Click(object sender, EventArgs e) {
    lbl.Text = q.Text;
}
</script>
</body>
</html>
