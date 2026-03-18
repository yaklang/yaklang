---
name: use-browser
description: >
  Browser automation skill for web page interaction. Covers opening URLs,
  taking accessibility snapshots with element refs, clicking, filling forms,
  typing text, taking screenshots, evaluating JavaScript, tab management,
  and navigating back/forward. Built on go-rod/CDP with headless Chrome.
  Used for web scraping, login automation, form submission, and UI testing.
---

# Browser Automation Skill (use_browser)

This skill teaches AI how to use the `use_browser` tool for browser automation tasks,
including web scraping, form filling, login flows, screenshot capture, and DOM interaction.

---

## 1. Prerequisites

The `use_browser` tool requires a Chrome or Chromium browser installed on the system.
If the browser is not available, the tool will report an error with installation instructions.

The tool uses the yaklang native `browser` module (go-rod/CDP) for all operations.
No external CLI tools or `exec` calls are used.

---

## 2. Core Workflow

The fundamental interaction pattern is:

```
open -> snapshot -> interact -> re-snapshot -> ... -> close
```

1. **Open**: Launch browser and navigate to a URL. This auto-takes a snapshot.
2. **Snapshot**: Returns an accessibility tree with interactive element refs (`@e1`, `@e2`, etc.).
3. **Interact**: Use refs to click, fill, or type on elements.
4. **Re-snapshot**: After any page change, take a new snapshot to refresh refs.
5. **Close**: Close the browser session when done.

> Element refs (`@eN`) are **invalidated** after any page navigation or significant DOM change.
> Always re-snapshot after clicks, form submissions, or navigation.

---

## 3. Operations Reference

### 3.1 Navigation Operations

| Operation | Required Params | Description |
|-----------|----------------|-------------|
| `open`    | `url`          | Open browser and navigate to URL. Auto-snapshots. |
| `back`    | -              | Navigate back in history. |
| `forward` | -              | Navigate forward in history. |
| `reload`  | -              | Reload current page. |
| `close`   | -              | Close browser session and release resources. |

### 3.2 Snapshot

| Operation  | Required Params | Description |
|------------|----------------|-------------|
| `snapshot` | -              | Take accessibility snapshot, returns element refs. |

Snapshot output is an accessibility tree like:

```
- RootWebArea "Example Domain"
  - heading "Example Domain"
  - paragraph "This domain is for use..."
  - link "More information..." @e1
```

Use `@e1` as the `target` for click/fill operations.

### 3.3 Interaction Operations

| Operation | Required Params       | Description |
|-----------|-----------------------|-------------|
| `click`   | `target`              | Click element by ref or CSS selector. Auto re-snapshots. |
| `fill`    | `target`, `value`     | Clear field and fill with text. For login forms, input fields. |
| `type`    | `value`               | Type text at current focus (keyboard input). No selector needed. |

### 3.4 Information Retrieval

| Operation | Required Params | Description |
|-----------|----------------|-------------|
| `get`     | `subop`        | Get page info. subop: `title`, `url`, `html`, `text`, `cookies`. |

### 3.5 Wait Operations

| Operation | Required Params            | Description |
|-----------|---------------------------|-------------|
| `wait`    | `wait-type`, `wait-value` | Wait for condition. Types: `selector`, `visible`, `ms`. |

### 3.6 Capture & Script

| Operation    | Required Params | Description |
|-------------|----------------|-------------|
| `screenshot` | -              | Take PNG screenshot and save to temp file. |
| `eval`       | `js`           | Execute JavaScript in page context. Use AITAG for multi-line JS. |

### 3.7 Tab Management

| Operation    | Required Params | Description |
|-------------|----------------|-------------|
| `tab_list`   | -              | List all open tabs with index, URL, title. |
| `tab_new`    | `url`          | Open new tab and navigate to URL. Auto-snapshots. |
| `tab_switch` | `tab-index`    | Switch to tab by index. |
| `tab_close`  | `tab-index`    | Close tab by index. |

---

## 4. Parameters Reference

| Parameter    | Type   | Default       | Description |
|-------------|--------|---------------|-------------|
| `op`        | string | (required)    | Operation name. See operations above. |
| `url`       | string | -             | URL for open/tab_new operations. |
| `target`    | string | -             | Element ref (@eN) or CSS selector for click/fill. |
| `value`     | string | -             | Text for fill/type operations. |
| `subop`     | string | -             | Sub-operation for `get`: title/url/html/text/cookies. |
| `js`        | string | -             | JavaScript for eval. Use AITAG for multi-line. |
| `session`   | string | `ai-browser`  | Session ID for browser instance reuse across tool calls. |
| `headless`  | string | `yes`         | `yes` or `no`. Set `no` to show browser window. |
| `timeout`   | int    | `30`          | Operation timeout in seconds. |
| `wait-type` | string | -             | Wait type: `selector`, `visible`, `ms`. |
| `wait-value`| string | -             | Wait value: CSS selector or milliseconds. |
| `tab-index` | string | -             | Tab index for tab_switch/tab_close. |

---

## 5. Common Patterns

### 5.1 Login Flow

```
1. open url=https://target.com/login
   -> snapshot shows: textbox "Username" @e1, textbox "Password" @e2, button "Login" @e3
2. fill target=@e1 value=admin
3. fill target=@e2 value=password123
4. click target=@e3
5. snapshot
   -> check if login succeeded by examining page content
```

### 5.2 Form Submission

```
1. open url=https://target.com/form
2. fill target=@e1 value="John Doe"
3. fill target=@e2 value="john@example.com"
4. click target=@e3 (submit button)
5. snapshot -> verify success page
```

### 5.3 Multi-Page Navigation

```
1. open url=https://target.com
2. click target=@e5 (a link)
3. snapshot -> inspect new page
4. get subop=url -> confirm current URL
5. back -> return to previous page
6. snapshot
```

### 5.4 Dynamic Content Handling

```
1. open url=https://target.com/spa
2. wait wait-type=selector wait-value=#dynamic-content
3. snapshot -> now dynamic content is loaded
4. click target=@e2
```

### 5.5 JavaScript Evaluation

```
1. open url=https://target.com
2. eval js="document.querySelectorAll('a').length"
   -> returns number of links
3. eval js="JSON.stringify(performance.timing)"
   -> returns page timing data
```

### 5.6 Screenshot for Evidence

```
1. open url=https://target.com/vulnerable-page
2. screenshot -> saves PNG with path
3. get subop=title -> record page title
```

---

## 6. Best Practices

1. **Always snapshot after open**: `open` auto-snapshots, but after any navigation (`click`, `back`, `forward`, `reload`), call `snapshot` explicitly to refresh refs.
2. **Use refs, not CSS selectors**: Prefer `@eN` refs from snapshots over raw CSS selectors. Refs are stable within a single snapshot.
3. **Set reasonable timeouts**: Use `timeout=10` for fast pages, `timeout=30` for slow ones.
4. **Wait for dynamic content**: Use `wait` before interacting with elements that load asynchronously.
5. **Close when done**: Always call `close` to release browser resources and avoid leaked processes.
6. **Check page state**: Use `get subop=title` or `get subop=url` to verify you are on the expected page before interacting.
7. **Handle errors gracefully**: If a click or fill fails, re-snapshot and inspect the page state.
8. **Session persistence**: The browser instance persists across tool calls using the same session ID. Default session is `ai-browser`.

---

## 7. Error Handling

| Error | Cause | Solution |
|-------|-------|----------|
| "No Chrome/Chromium browser found" | Chrome not installed | Install Chrome or Chromium |
| "no browser instance found" | Session not opened yet | Call `open` first |
| "no page found in session" | No active page | Navigate to a URL first |
| "click @eN failed" | Ref invalidated or not found | Re-snapshot and use fresh refs |
| "fill @eN failed" | Element not fillable | Verify target is an input/textarea |
| "navigate failed" | Network error or timeout | Check URL and increase timeout |

---

## 8. Tool Call Format

```json
{
  "@action": "call-tool",
  "tool": "use_browser",
  "identifier": "descriptive_action_name",
  "params": {
    "op": "open",
    "url": "https://example.com",
    "timeout": 15
  }
}
```

For multi-line JavaScript in `eval`, use AITAG:

```json
{
  "@action": "call-tool",
  "tool": "use_browser",
  "identifier": "eval_script",
  "params": {
    "op": "eval",
    "timeout": 10
  }
}
```

```
<|TOOL_PARAM_js_{NONCE}|>
JSON.stringify({
  title: document.title,
  links: Array.from(document.querySelectorAll("a")).map(a => a.href)
})
<|TOOL_PARAM_js_END_{NONCE}|>
```
