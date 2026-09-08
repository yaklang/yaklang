<%@ Page Language="C#" %>
<asp:Repeater ID="list" runat="server">
<ItemTemplate>
<span><%# Eval("Name") %></span>
</ItemTemplate>
</asp:Repeater>
