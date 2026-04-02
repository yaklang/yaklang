<%
Calendar curCal = new GregorianCalendar(timeZone, locale);
String calendar_num = request.getParameter("calendar_num");
int cal_num = Integer.parseInt(calendar_num);

	for (int i=0;i<cal_num;i++) {
%>
	// Calendar Stuff
	var <portlet:namespace />calObj_<%=i%> = new Calendar(false, null, <portlet:namespace />calendarOnSelect_<%=i%>, <portlet:namespace />calendarOnClose);
	<portlet:namespace />calObj_<%=i%>.weekNumbers = false;
	<portlet:namespace />calObj_<%=i%>.firstDayOfWeek = <%= curCal.getFirstDayOfWeek() - 1 %>;
<%
	}
%>
