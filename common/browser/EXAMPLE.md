# browser 模块使用示例

以下每个示例都是完整可运行的 yak 脚本。目标服务假设运行在 `http://127.0.0.1:8787`（Vulinbox）。

## 示例 1: 基础操作 -- 打开/导航/获取信息/关闭

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

page, err = b.Navigate("http://127.0.0.1:8787/")
assert err == nil, sprintf("navigate failed: %v", err)

title, _ = page.Title()
url = page.URL()
html, _ = page.HTML()

log.info("title: %v", title)
log.info("url: %v", url)
log.info("html length: %v bytes", len(html))
```

## 示例 2: Snapshot + Ref 交互

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

page, err = b.Navigate("http://127.0.0.1:8787/")
assert err == nil, sprintf("navigate failed: %v", err)

// 获取 accessibility tree snapshot
snap, err = page.Snapshot()
assert err == nil, sprintf("snapshot failed: %v", err)

log.info("nodes: %v, refs: %v", snap.NodeCount, snap.RefMap.Count())
log.info("snapshot:\n%v", snap.Text)

// 通过 ref 填写文本框
// 从 snapshot 中可以看到: textbox "快速过滤筛选案例" [ref=e5]
err = page.Fill("@e5", "SQL")
if err == nil {
    log.info("filled textbox @e5 with 'SQL'")
}

// 通过 ref 点击链接
// 从 snapshot 中可以看到: link "Vulinbox - Agent" [ref=e1]
err = page.Click("@e1")
if err == nil {
    log.info("clicked link @e1")
    sleep(0.5)
    log.info("navigated to: %v", page.URL())
}

// 点击后重新获取 snapshot（ref 会重新分配）
snap2, err = page.Snapshot()
if err == nil {
    log.info("new page refs: %v", snap2.RefMap.Count())
}
```

## 示例 3: CSS 选择器操作

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

page, err = b.Navigate("http://127.0.0.1:8787/")
assert err == nil, sprintf("navigate failed: %v", err)

// 查找单个元素
body, err = page.Element("body")
assert err == nil, sprintf("find body failed: %v", err)

text, _ = body.Text()
log.info("body text: %v chars", len(text))

// 查找多个元素
links, err = page.Elements("a")
assert err == nil, sprintf("find links failed: %v", err)
log.info("found %v links", len(links))

// 遍历元素
for i, link = range links {
    if i >= 5 { break }
    t, _ = link.Text()
    href, _ = link.Attribute("href")
    log.info("  link[%v]: text=%v href=%v", i, t, href)
}

// 通过 CSS 选择器填写和点击
err = page.Fill("input", "test-value")
if err == nil {
    log.info("filled input via css selector")
}

err = page.Click("a")
if err == nil {
    log.info("clicked first <a> via css selector")
}
```

## 示例 4: 命名多实例并行

```yak
defer browser.CloseAll()

// 同时打开三个独立的浏览器实例
b1, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open default failed: %v", err)

b2, err = browser.Open(browser.id("scanner"), browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open scanner failed: %v", err)

b3, err = browser.Open(browser.id("crawler"), browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open crawler failed: %v", err)

// 各自独立导航
p1, _ = b1.Navigate("http://127.0.0.1:8787/")
p2, _ = b2.Navigate("http://127.0.0.1:8787/")
p3, _ = b3.Navigate("http://127.0.0.1:8787/misc/healthy")

// 列出所有实例
ids = browser.List()
log.info("active instances: %v", ids)

// 各自独立操作
t1, _ = p1.Title()
t2, _ = p2.Title()
t3, _ = p3.Title()
log.info("default title: %v", t1)
log.info("scanner title: %v", t2)
log.info("crawler title: %v", t3)

// 按需关闭
browser.Close(browser.id("crawler"))
log.info("after closing crawler: %v", browser.List())

// 最终清理
browser.CloseAll()
log.info("after closeAll: %v", browser.List())
```

## 示例 5: 跨阶段 Get 复用

```yak
defer browser.CloseAll()

// 阶段 1: 创建浏览器并登录
func setupBrowser() {
    b, err = browser.Open(browser.id("session"), browser.headless(true), browser.timeout(10))
    assert err == nil, sprintf("open failed: %v", err)
    page, _ = b.Navigate("http://127.0.0.1:8787/")
    log.info("setup: navigated to %v", page.URL())
}

// 阶段 2: 在其他地方获取同一个浏览器继续操作
func useBrowser() {
    b, err = browser.Get(browser.id("session"))
    assert err == nil, sprintf("get session failed: %v", err)
    page, _ = b.CurrentPage()
    log.info("use: current page url = %v", page.URL())
    
    snap, _ = page.Snapshot()
    log.info("use: page has %v refs", snap.RefMap.Count())
}

setupBrowser()
useBrowser()
```

## 示例 6: Tab 管理

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

// 打开第一个标签页
page1, _ = b.Navigate("http://127.0.0.1:8787/")
log.info("tab 1 url: %v", page1.URL())

// 打开第二个标签页
page2, _ = b.NewTab("http://127.0.0.1:8787/misc/healthy")
log.info("tab 2 url: %v", page2.URL())

// 列出所有标签页
tabs, _ = b.ListTabs()
log.info("total tabs: %v", len(tabs))
for _, tab = range tabs {
    log.info("  [%v] %v - %v", tab["index"], tab["url"], tab["title"])
}

// 切换到第一个标签页
switched, _ = b.SwitchTab(0)
log.info("switched to tab 0: %v", switched.URL())

// 关闭最后一个标签页
b.CloseTab(len(tabs) - 1)
tabs2, _ = b.ListTabs()
log.info("tabs after close: %v", len(tabs2))
```

## 示例 7: JavaScript 执行

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

page, _ = b.Navigate("http://127.0.0.1:8787/")

// 获取页面属性
title, _ = page.Evaluate("document.title")
log.info("document.title: %v", title)

href, _ = page.Evaluate("window.location.href")
log.info("location.href: %v", href)

// 执行计算
result, _ = page.Evaluate("2 + 3 * 4")
log.info("2 + 3 * 4 = %v", result)

// DOM 查询
linkCount, _ = page.Evaluate("document.querySelectorAll('a').length")
log.info("total links: %v", linkCount)

childCount, _ = page.Evaluate("document.body.children.length")
log.info("body children: %v", childCount)

// 字符串拼接
greeting, _ = page.Evaluate("'hello' + ' ' + 'world'")
log.info("greeting: %v", greeting)
```

## 示例 8: 截图

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

page, _ = b.Navigate("http://127.0.0.1:8787/")

// 获取 PNG 字节数据
imgData, err = page.Screenshot()
assert err == nil, sprintf("screenshot failed: %v", err)
log.info("screenshot: %v bytes", len(imgData))

// 验证 PNG 格式
assert imgData[0] == 0x89 && imgData[1] == 0x50, "should be PNG format"

// 获取 Base64 Data URI（适合嵌入 HTML 或传给 AI）
dataURI, err = page.ScreenshotBase64()
assert err == nil, sprintf("screenshotBase64 failed: %v", err)
log.info("base64 URI length: %v chars", len(dataURI))
assert str.HasPrefix(dataURI, "data:image/png;base64,"), "should start with data URI prefix"
```

## 示例 9: AI Agent 循环模式

这是 browser 模块区别于传统 crawler 的核心使用场景：snapshot -> AI 分析 -> ref 操作 循环。

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

page, err = b.Navigate("http://127.0.0.1:8787/")
assert err == nil, sprintf("navigate failed: %v", err)

// helper: 从 snapshot 中提取指定 role 的第一个 ref
extractRef = func(snapText, role) {
    lines = str.Split(snapText, "\n")
    for _, line = range lines {
        if str.Contains(line, role) && str.Contains(line, "ref=") {
            parts = str.SplitN(line, "ref=", 2)
            if len(parts) < 2 { continue }
            refPart = parts[1]
            result = ""
            for i = 0; i < len(refPart); i++ {
                ch = sprintf("%c", refPart[i])
                if ch == "]" || ch == "," || ch == " " { break }
                result = result + ch
            }
            if result != "" { return result }
        }
    }
    return ""
}

// AI Agent 循环
maxSteps = 3
for step = 0; step < maxSteps; step++ {
    log.info("--- agent step %v ---", step + 1)
    
    // 1. 获取当前页面快照
    snap, err = page.Snapshot()
    if err != nil {
        log.info("snapshot error: %v", err)
        break
    }
    log.info("page: %v nodes, %v refs", snap.NodeCount, snap.RefMap.Count())
    
    // 2. AI 分析 snapshot.Text 并决策（这里模拟 AI 行为）
    // 实际场景中，将 snap.Text 发送给 AI 模型，获取操作指令
    excerpt = snap.Text
    if len(excerpt) > 300 { excerpt = excerpt[:300] }
    log.info("snapshot excerpt:\n%v", excerpt)
    
    // 3. 根据 AI 决策执行操作
    if step == 0 {
        // 模拟 AI 决定填写搜索框
        ref = extractRef(snap.Text, "textbox")
        if ref != "" {
            log.info("AI decision: fill textbox @%v", ref)
            page.Fill("@" + ref, "XSS")
        }
    } else if step == 1 {
        // 模拟 AI 决定点击某个链接
        ref = extractRef(snap.Text, "link ")
        if ref != "" {
            log.info("AI decision: click link @%v", ref)
            page.Click("@" + ref)
            sleep(0.5)
        }
    } else {
        log.info("AI decision: task complete")
        break
    }
}

log.info("agent finished after %v steps", maxSteps)
```

## 示例 10: 综合端到端测试

```yak
defer browser.CloseAll()

targetURL = "http://127.0.0.1:8787/"

log.info("=== phase 1: lifecycle ===")
b, err = browser.Open(browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)
assert b.ID() == "default"
assert b.IsClosed() == false

log.info("=== phase 2: navigate ===")
page, err = b.Navigate(targetURL)
assert err == nil, sprintf("navigate failed: %v", err)
assert str.Contains(page.URL(), "127.0.0.1:8787")
title, _ = page.Title()
assert len(title) > 0

log.info("=== phase 3: snapshot ===")
snap, err = page.Snapshot()
assert err == nil, sprintf("snapshot failed: %v", err)
assert snap.NodeCount > 0
assert snap.RefMap.Count() > 0
assert str.Contains(snap.Text, "ref=e")

log.info("=== phase 4: ref interaction ===")
extractRef = func(snapText, role) {
    lines = str.Split(snapText, "\n")
    for _, line = range lines {
        if str.Contains(line, role) && str.Contains(line, "ref=") {
            parts = str.SplitN(line, "ref=", 2)
            if len(parts) < 2 { continue }
            refPart = parts[1]
            result = ""
            for i = 0; i < len(refPart); i++ {
                ch = sprintf("%c", refPart[i])
                if ch == "]" || ch == "," || ch == " " { break }
                result = result + ch
            }
            if result != "" { return result }
        }
    }
    return ""
}

textboxRef = extractRef(snap.Text, "textbox")
if textboxRef != "" {
    err = page.Fill("@" + textboxRef, "e2e-test")
    assert err == nil, sprintf("fill ref failed: %v", err)
}

log.info("=== phase 5: evaluate ===")
jsResult, _ = page.Evaluate("1 + 1")
log.info("1 + 1 = %v", jsResult)

log.info("=== phase 6: screenshot ===")
imgData, err = page.Screenshot()
assert err == nil, sprintf("screenshot failed: %v", err)
assert len(imgData) > 100

log.info("=== phase 7: multi-instance ===")
b2, err = browser.Open(browser.id("second"), browser.headless(true), browser.timeout(10))
assert err == nil, sprintf("open second failed: %v", err)

ids = browser.List()
assert len(ids) == 2

log.info("=== phase 8: tabs ===")
page2, _ = b.NewTab(targetURL)
tabs, _ = b.ListTabs()
assert len(tabs) >= 2

log.info("=== phase 9: cleanup ===")
browser.CloseAll()
assert len(browser.List()) == 0

log.info("=== E2E TEST PASSED ===")
```

## 示例 11: 跨进程复用 -- 启动并保留

脚本 A 启动浏览器，保存 controlURL 到文件，退出时不关闭 Chrome。

```yak
// save as: step1_open.yak
// DO NOT close browser

b, err = browser.Open(browser.headless(true), browser.leakless(false), browser.timeout(10))
assert err == nil, sprintf("open failed: %v", err)

page, _ = b.Navigate("http://127.0.0.1:8787/")
log.info("navigated to %v", page.URL())

snap, _ = page.Snapshot()
log.info("page has %v refs", snap.RefMap.Count())

ctrlURL = b.ControlURL()
log.info("controlURL: %v", ctrlURL)
file.Save("/tmp/ctrl_url.txt", ctrlURL)
log.info("controlURL saved, browser left running")
```

## 示例 12: 跨进程复用 -- 重连并操作

脚本 B 在另一个进程中读取 controlURL，重连到同一个 Chrome，继续操作。

```yak
// save as: step2_operate.yak
// DO NOT close browser

ctrlURL = str.TrimSpace(string(file.ReadFile("/tmp/ctrl_url.txt")~))
b, err = browser.Open(browser.controlURL(ctrlURL), browser.timeout(10))
assert err == nil, sprintf("reconnect failed: %v", err)

tabs, _ = b.ListTabs()
log.info("found %v tabs from previous script", len(tabs))
for _, tab = range tabs {
    log.info("  %v - %v", tab["url"], tab["title"])
}

page, _ = b.CurrentPage()
snap, _ = page.Snapshot()
log.info("snapshot: %v nodes", snap.NodeCount)

page2, _ = b.NewTab("http://127.0.0.1:8787/misc/healthy")
log.info("opened new tab: %v", page2.URL())
log.info("browser still running for next script")
```

## 示例 13: 跨进程复用 -- 验证并关闭

脚本 C 重连，验证前两个脚本的状态，然后关闭。

```yak
// save as: step3_close.yak

ctrlURL = str.TrimSpace(string(file.ReadFile("/tmp/ctrl_url.txt")~))
b, _ = browser.Open(browser.controlURL(ctrlURL), browser.timeout(10))

tabs, _ = b.ListTabs()
log.info("accumulated tabs: %v", len(tabs))

b.Close()
os.Remove("/tmp/ctrl_url.txt")
log.info("browser closed and cleanup done")
```

## 示例 14: AI 模拟登录

模拟 AI Agent 探索登录页面并尝试登录（非 headless，可视观察）。

```yak
defer browser.CloseAll()

b, err = browser.Open(browser.headless(false), browser.timeout(15))
assert err == nil, sprintf("open failed: %v", err)

page, err = b.Navigate("http://127.0.0.1:8787/logic/user/login")
assert err == nil, sprintf("navigate failed: %v", err)
sleep(1)

// AI step 1: 获取页面结构
snap, _ = page.Snapshot()
log.info("page: %v nodes, %v refs", snap.NodeCount, snap.RefMap.Count())
log.info("snapshot:\n%v", snap.Text)

// AI step 2: 通过 snapshot 或 CSS 选择器找到输入框
// 填写用户名和密码
page.Fill("input[type=text]", "admin")
sleep(0.3)
page.Fill("input[type=password]", "admin")
sleep(0.3)

// AI step 3: 点击登录按钮
page.Click("button")
sleep(2)

// AI step 4: 观察结果
log.info("url after login: %v", page.URL())
jsResult, _ = page.Evaluate("document.body.innerText.substring(0, 300)")
log.info("page text: %v", jsResult)
```
