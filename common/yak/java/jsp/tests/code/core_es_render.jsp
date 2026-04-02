<%if( LicenseUtil.getLevel() < LicenseLevel.STANDARD.level){ %>
	<div class="portlet-wrapper">
		<jsp:include page="/WEB-INF/jsp/es-search/not_licensed.jsp"></jsp:include>
	</div>
<%return;}%>

<table class="listingTable" style="width:90%;">
	<tr>
		<th><label for="live"><%= LanguageUtil.get(pageContext, "Live") %></label></th>
		<td nowrap="nowrap">
			<input dojoType="dijit.form.CheckBox" name="live"  id="live" type="checkbox" value="true" <%=live ? "checked=true" : ""%>
		</td>
	</tr>
</table>

<div style='text-align:center;padding:20px;'>
	<button type="button" id="submitButton"  iconClass="queryIcon" onClick="refreshPane()" dojoType="dijit.form.Button" value="Submit"><%= LanguageUtil.get(pageContext, "Query") %></button>
</div>

<%if(UtilMethods.isSet(cons)){ %>
	<table class="listingTable" style="width:90%;">
		<tr>
			<th nowrap="nowrap"><%= LanguageUtil.get(pageContext, "Showing Hits") %></th>
			<td><%=cons.getCount() %> of <%=cons.getTotalResults()%></td>
		</tr>
		<tr>
			<th><%= LanguageUtil.get(pageContext, "Took") %></th>
			<td style="width:100%">
				<%=cons.getQueryTook()%> ms <%= LanguageUtil.get(pageContext, "query") %><br>
				<%=cons.getPopulationTook()%> ms <%= LanguageUtil.get(pageContext, "population") %><br>
			</td>
		</tr>
	</table>
<%}%>
