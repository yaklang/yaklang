<#if obj.primaryKey != prop.name>
<if test="${prop.name} != null">
    <#if prop.name = "createdDate">
    <#if (prop_index > 1)>and </#if>${prop.columnName} &gt;= #${_brack}${prop.name}}
    <!--and ${r"#{createdDate_fan1}"} -->
    <#else>
    <#if (prop_index > 1)>and </#if>${prop.columnName} = #${_brack}${prop.name}}
    </#if>
</if>
</#if>
