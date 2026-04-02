<html xmlns="http://www.w3.org/1999/xhtml">
<div dojoType="dijit.layout.ContentPane" splitter="true" region="center" class="portlet-content-search" id="contentWrapper" style="overflow-y:auto; overflow-x:auto;">
	<div class="portlet-sidebar">
		<dl class="vertical">
			<dt><label for="userId"><%= LanguageUtil.get(pageContext, "UserID") %> </label></dt>
			<dd><input name="userId" id="userId" dojoType="dijit.form.TextBox" type="text" value="<%=UtilMethods.webifyString(userToPullID)%>" size="40" <% if(!userIsAdmin){ %> disabled="disabled" <% } %>/></dd>
		</dl>
	</div>

	<div class="portlet-main" id="luceneResultContainer" style="margin: 35px 20px;">
		<table class="listingTable">
			<tr>
				<td><strong><%= LanguageUtil.get(pageContext, "The-total-results-are") %> :</strong> <span id="luceneResultSize">0</span></td>
				<td></td>
			</tr>
			<tr>
				<td><strong><%= LanguageUtil.get(pageContext, "Query-took") %> :</strong> <<span id="luceneQueryTook">0</span> ms</td>
				<td><em><%= LanguageUtil.get(pageContext, "This-includes-permissions-but-returns-only-the-index-objects") %></em></td>
			</tr>
			<tr>
				<td><strong><%= LanguageUtil.get(pageContext, "Content-Population-took") %> :</strong> <span id="luceneContentPopulation">0</span> ms</td>
				<td><em><%= LanguageUtil.get(pageContext, "This-includes-permissions-and-returns-full-content-objects") %></em></td>
			</tr>
		</table>
	</div>
</div>
</html>
