<%@ Page Language="C#" %>
<!-- Typical unquoted <%= %> in an attribute (ASP blob in ATTVALUE mode). -->
<a href=<%= Request["next"] %> class="next">next</a>
<img src=<%= img %> alt="x" />
