package tests

import (
	"github.com/yaklang/yaklang/common/yak/java/freemarker"
	"testing"
)

func TestGetAST(t *testing.T) {
	result, err := freemarker.GetAST(`<#-- Freemarker 模板用法示例 -->

<#-- 注释：单行注释 -->

<#-- 包含宏定义文件 -->
<#include "macros.ftl">

<#-- 变量定义 -->
<#assign myString = "Hello, World!">
<#assign myNumber = 42>
<#assign myBoolean = true>
<#assign myList = ["apple", "banana", "cherry"]>
<#assign myMap = {"name": "Alice", "age": 30}>

<!DOCTYPE html>
<html>
<head>
    <title>FreeMarker 模板用法示例</title>
</head>
<body>
    <h1>FreeMarker 模板用法示例</h1>

    <#-- 变量插值 -->
    <p>${myString}</p>

    <#-- IF-ELSE 指令 -->
    <#if myNumber == 42>
        <p>The number is 42.</p>
    <#elseif myNumber != 42>
        <p>The number is not 42.</p>
    <#else>
        <p>What is the number?</p>
    </#if>

    <#-- 循环 -->
    <h2>列表循环</h2>
    <ul>
        <#list myList as item>
            <li>${item}</li>
        </#list>
    </ul>

    <#-- Map遍历 -->
    <h2>Map遍历</h2>
    <#list myMap?keys as key>
        <p>${key}:${myMap[key]}</p>
    </#list>

    <#-- 内建函数 -->
    <h2>内建函数</h2>
    <p>字符串长度：${myString?length}</p>
    <p>数字格式化：${myNumber?string["0.00"]}</p>

    <#-- 使用宏 -->
    <@renderProduct product=myMap />

    <#-- 嵌套循环 -->
    <h2>嵌套循环</h2>
    <table>
        <#list myList as item>
            <tr>
                <#list item?split("") as char>
                    <td>${char}</td>
                </#list>
            </tr>
        </#list>
    </table>

    <#-- 使用命名空间 -->
    <h2>命名空间</h2>
    <@myNamespace.renderMessage message="Hello from a namespace!" />

    <#-- switch-case 模拟 -->
    <h2>switch-case 模拟</h2>
    <#assign number = 2>
    <#switch number>
        <#case 1>
            Number is 1.
        <#case 2>
            Number is 2.
        <#case 3>
            Number is 3.
        <#default>
            Number is not 1, 2, or 3.
    </#switch>

    <#-- 使用函数 -->
    <h2>自定义函数</h2>
    <p>${myCustomFunction("Hello")}</p>

    <#-- 处理空值 -->
    <h2>处理空值</h2>
    <#assign nullableValue = "">
    <#if nullableValue??>
        <p>Value is not null.</p>
    <#else>
        <p>Value is null.</p>
    </#if>

    <#-- 日期格式化 -->
    <h2>日期格式化</h2>
    <#assign now = .now>
    <p>${now?string["yyyy-MM-dd HH:mm:ss"]}</p>

</body>
</html>
`)
	if err != nil {
		t.Fatal(err)
	}
	template := result.Template()
	if template == nil {
		t.Fatal("freeMarker is nil")
	}
}
