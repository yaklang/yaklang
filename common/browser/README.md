# browser -- AI-First Browser Automation Module

`browser` 是 yaklang 中面向 AI 场景的浏览器自动化模块。核心创新在于 **Accessibility Tree Snapshot + Element Ref 系统**，使 AI Agent 无需依赖 CSS 选择器即可理解页面结构并操作交互元素。

与传统的 `simulator.simple` / `crawlerx` 不同，`browser` 模块以 AI 可读性为第一优先级设计，同时保留完整的 CSS 选择器操作能力。

## 架构

```
BrowserManager (globalManager)
  |
  +-- browsers["default"]   --> BrowserInstance
  |                              |-- *rod.Browser
  |                              |-- pages []*BrowserPage
  |                              |       |-- *rod.Page
  |                              |       |-- *RefMap (ref -> backendNodeId)
  |                              |       +-- timeout
  |                              +-- *BrowserConfig
  |
  +-- browsers["scanner"]   --> BrowserInstance ...
  +-- browsers["crawler"]   --> BrowserInstance ...
```

`BrowserManager` 是全局命名实例池。每个 `BrowserInstance` 管理一个独立的 Chrome 进程，可包含多个 `BrowserPage`（标签页）。每个 `BrowserPage` 拥有独立的 `RefMap`，在每次 `Snapshot()` 调用时重新生成。

## 快速开始

```yak
b, err = browser.Open(browser.headless(true), browser.timeout(10))
page, err = b.Navigate("http://example.com")
snap, err = page.Snapshot()
println(snap.Text)
browser.CloseAll()
```

## 完整 API 参考

### 顶层函数

通过 `browser.` 前缀在 yak 脚本中调用。

#### browser.Open(opts...) -- 创建或获取浏览器实例

同一 ID 已有活跃实例时直接返回（不重复创建）。

```yak
b, err = browser.Open(browser.headless(true))
b, err = browser.Open(browser.id("scanner"), browser.headless(true), browser.timeout(15))
```

#### browser.Get(opts...) -- 获取已存在的实例

实例不存在或已关闭时返回 error。

```yak
b, err = browser.Get()
b, err = browser.Get(browser.id("scanner"))
```

#### browser.List() -- 列出所有活跃实例 ID

```yak
ids = browser.List()
// ["default", "scanner"]
```

#### browser.Close(opts...) -- 关闭指定实例

```yak
err = browser.Close()
err = browser.Close(browser.id("scanner"))
```

#### browser.CloseAll() -- 关闭所有实例

```yak
browser.CloseAll()
```

### 配置选项

所有选项作为 `browser.Open` 的参数传入。

| 选项 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `browser.id(string)` | string | `"default"` | 实例 ID，用于命名管理 |
| `browser.headless(bool)` | bool | `true` | 无头模式 |
| `browser.proxy(string)` | string | `""` | HTTP 代理地址 |
| `browser.timeout(float64)` | float64(秒) | `30` | 操作超时时间 |
| `browser.exePath(string)` | string | 自动检测 | Chrome 可执行文件路径 |
| `browser.controlURL(string)` | string | `""` | 直连已运行 Chrome 的 WebSocket URL（跨进程复用） |
| `browser.wsAddress(string)` | string | `""` | 远程 managed launcher 地址 |
| `browser.noSandBox(bool)` | bool | `true` | 禁用沙箱（Linux 需要） |
| `browser.leakless(bool)` | bool | `false` | 防泄漏模式 |

### BrowserInstance 方法

通过 `browser.Open()` 或 `browser.Get()` 返回的实例调用。

#### b.Navigate(url) -- 创建新标签页并导航

```yak
page, err = b.Navigate("http://example.com")
```

#### b.CurrentPage() -- 获取当前活跃页面

```yak
page, err = b.CurrentPage()
```

#### b.ListTabs() -- 列出所有标签页

```yak
tabs, err = b.ListTabs()
for _, tab = range tabs {
    println(tab["index"], tab["url"], tab["title"])
}
```

#### b.NewTab(url) -- 打开新标签页

```yak
page, err = b.NewTab("http://example.com/page2")
```

#### b.SwitchTab(index) -- 切换到指定标签页

```yak
page, err = b.SwitchTab(0)
```

#### b.CloseTab(index) -- 关闭指定标签页

```yak
err = b.CloseTab(1)
```

#### b.Close() -- 关闭浏览器实例

```yak
err = b.Close()
```

#### b.ID() / b.IsClosed() / b.ControlURL()

```yak
id = b.ID()              // "default"
closed = b.IsClosed()     // false
ctrlURL = b.ControlURL()  // "ws://127.0.0.1:9222/devtools/browser/..."
```

### BrowserPage 方法

通过 `b.Navigate()` 或 `b.CurrentPage()` 返回的页面调用。

#### 导航

```yak
err = page.Navigate("http://example.com/new")
err = page.NavigateAndWait("http://example.com/new", "h1")
err = page.Reload()
err = page.Back()
err = page.Forward()
```

#### 交互 -- 支持 @ref 和 CSS 选择器

```yak
// 通过 ref 操作（需先调用 Snapshot）
err = page.Click("@e1")
err = page.Fill("@e3", "search query")

// 通过 CSS 选择器操作
err = page.Click("#submit-btn")
err = page.Fill("input[name=q]", "text")

// 直接输入文本（输入到当前焦点元素）
err = page.Type("hello")
```

#### Snapshot -- 核心功能

```yak
snap, err = page.Snapshot()
println(snap.Text)        // accessibility tree 文本
println(snap.NodeCount)   // 节点总数
println(snap.RefMap.Count()) // 可交互元素引用数
```

#### 信息获取

```yak
html, err = page.HTML()
title, err = page.Title()
url = page.URL()
result, err = page.Evaluate("document.title")
```

#### 截图

```yak
imgBytes, err = page.Screenshot()           // []byte (PNG)
dataURI, err = page.ScreenshotBase64()      // "data:image/png;base64,..."
```

#### 等待

```yak
err = page.WaitSelector("div.loaded")
err = page.WaitVisible("#content")
```

#### 元素查找

```yak
el, err = page.Element("h1")
els, err = page.Elements("a")
```

#### Cookie

```yak
cookies, err = page.GetCookies()
err = page.SetCookies(cookies)
```

### BrowserElement 方法

通过 `page.Element()` 或 `page.Elements()` 返回的元素调用。

```yak
text, err = el.Text()
html, err = el.HTML()
value, err = el.Attribute("href")
err = el.Click()
err = el.Input("text")
err = el.Focus()
visible, err = el.Visible()
err = el.WaitVisible()
```

### SnapshotResult 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `Text` | string | accessibility tree 的文本渲染 |
| `RefMap` | *RefMap | 元素引用映射表 |
| `NodeCount` | int | accessibility tree 节点总数 |

## Ref 系统

### 什么是 Ref

Ref 是为可交互元素分配的短标识符（如 `@e1`、`@e2`），AI Agent 可以直接使用这些引用操作页面元素，无需构造 CSS 选择器。

### Snapshot 输出格式

```
- RootWebArea
  - navigation
    - link "Home" [ref=e1]
    - link "About" [ref=e2]
  - main
    - heading "Welcome" [level=1]
    - textbox "Search" [ref=e3]
    - button "Submit" [ref=e4]
```

### 使用 Ref 操作元素

```yak
snap, err = page.Snapshot()
// AI 分析 snap.Text，决定操作
err = page.Fill("@e3", "search query")
err = page.Click("@e4")
```

### 支持的 Ref 格式

- `@e1` -- 标准格式
- `ref=e1` -- 兼容格式

### 自动分配 Ref 的元素类型

以下 17 种 interactive role 的元素会被自动分配 ref：

button, link, textbox, combobox, checkbox, radio, switch, slider, spinbutton, tab, menuitem, option, treeitem, searchbox, menuitemcheckbox, menuitemradio

### AI Agent 使用模式

```yak
b, err = browser.Open(browser.headless(true))
page, err = b.Navigate(targetURL)

for {
    snap, err = page.Snapshot()
    // 将 snap.Text 发送给 AI 分析
    // AI 返回操作指令，如 "click @e3" 或 "fill @e5 with 'test'"
    // 解析指令并执行
    err = page.Click("@e3")
    // 循环直到任务完成
}
```

## 跨进程复用

通过 `controlURL` 实现多个 yak 脚本进程共享同一个 Chrome 实例：

```yak
// 脚本 A: 启动浏览器，保存 controlURL
b, err = browser.Open(browser.headless(true), browser.leakless(false))
page, _ = b.Navigate("http://example.com")
file.Save("/tmp/ctrl_url.txt", b.ControlURL())
// 脚本退出，不调用 Close，Chrome 继续运行
```

```yak
// 脚本 B (另一个进程): 通过 controlURL 重连
ctrlURL = string(file.ReadFile("/tmp/ctrl_url.txt")~)
b, err = browser.Open(browser.controlURL(ctrlURL))
tabs, _ = b.ListTabs()   // 能看到脚本 A 打开的页面
page, _ = b.CurrentPage() // 获取脚本 A 的页面继续操作
```

```yak
// 脚本 C: 重连并关闭
ctrlURL = string(file.ReadFile("/tmp/ctrl_url.txt")~)
b, _ = browser.Open(browser.controlURL(ctrlURL))
b.Close()  // 真正关闭 Chrome 进程
```

## 测试

测试脚本位于 `common/browser/yaktests/`，覆盖全部 API：

### 基础测试（独立运行）

```bash
go run common/yak/cmd/yak.go common/browser/yaktests/01_lifecycle.yak
go run common/yak/cmd/yak.go common/browser/yaktests/02_navigate.yak
go run common/yak/cmd/yak.go common/browser/yaktests/03_snapshot.yak
go run common/yak/cmd/yak.go common/browser/yaktests/04_ref_interaction.yak
go run common/yak/cmd/yak.go common/browser/yaktests/05_selector.yak
go run common/yak/cmd/yak.go common/browser/yaktests/06_evaluate.yak
go run common/yak/cmd/yak.go common/browser/yaktests/07_screenshot.yak
go run common/yak/cmd/yak.go common/browser/yaktests/08_multi_instance.yak
go run common/yak/cmd/yak.go common/browser/yaktests/09_tabs.yak
```

### 跨进程测试（必须顺序运行）

```bash
# Headless 跨进程三部曲
go run common/yak/cmd/yak.go common/browser/yaktests/10_cross_step1_open.yak
go run common/yak/cmd/yak.go common/browser/yaktests/11_cross_step2_operate.yak
go run common/yak/cmd/yak.go common/browser/yaktests/12_cross_step3_close.yak

# GUI 跨进程三部曲（可见浏览器窗口）
go run common/yak/cmd/yak.go common/browser/yaktests/13_gui_step1_open.yak
go run common/yak/cmd/yak.go common/browser/yaktests/14_gui_step2_operate.yak
go run common/yak/cmd/yak.go common/browser/yaktests/15_gui_step3_close.yak
```

### AI 模拟测试

```bash
go run common/yak/cmd/yak.go common/browser/yaktests/16_ai_login.yak
```

| 脚本 | 覆盖范围 |
|------|----------|
| 01_lifecycle | Open / Get / List / Close / CloseAll / IsClosed |
| 02_navigate | Navigate / Title / URL / HTML / Reload |
| 03_snapshot | Snapshot / Text / NodeCount / RefMap.Count |
| 04_ref_interaction | Click(@ref) / Fill(@ref) / Snapshot 重新生成 |
| 05_selector | Click(css) / Fill(css) / Element / Elements / BrowserElement |
| 06_evaluate | Evaluate JS 表达式 |
| 07_screenshot | Screenshot / ScreenshotBase64 |
| 08_multi_instance | 多 ID 并行 / Get 复用 / 分别关闭 |
| 09_tabs | ListTabs / NewTab / SwitchTab / CloseTab |
| 10-12 | 跨进程 headless: 启动/重连操作/验证关闭 |
| 13-15 | 跨进程 GUI: 同上但可见窗口 |
| 16 | AI 模拟登录: snapshot 探索 + 填表 + 提交 |

## 使用示例

完整的场景示例参见 [EXAMPLE.md](EXAMPLE.md)。
