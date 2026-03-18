---
name: how-to-use-browser
description: >
  Browser automation skill for web page interaction. Two strategies:
  (1) Snapshot + refs for simple static pages;
  (2) JavaScript-first for login forms, SPA, and dynamic pages (PREFERRED).
  Covers opening URLs, snapshots, clicking, filling forms, evaluating JS,
  screenshots, tab management, and navigation. Built on go-rod/CDP with headless Chrome.
  If snapshot returns 0 element refs, DO NOT retry -- switch to JavaScript strategy immediately.
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

## 2.5 JavaScript-First Strategy (RECOMMENDED for Login/Form/SPA Pages)

For login pages, forms, and SPA (React/Vue/Angular) pages, use **JavaScript evaluation**
instead of the snapshot+refs workflow. This is more reliable because:

1. SPA pages often render interactive elements via JavaScript, making snapshot return 0 refs.
2. A single `eval` call can fill a form and submit it -- more efficient than multiple tool calls.
3. CSS selectors from JS work directly with `fill` and `click` operations.

### When to use JS-first:
- Login forms (username/password + submit)
- Any page where snapshot returns 0 interactive refs
- Complex forms with dynamic validation
- Pages that require JavaScript to render

### JS-first workflow:
```
open -> eval JS to discover form fields -> fill/click with CSS selectors (or do everything in one eval)
```

### Step 1: Discover form fields
```
eval js: JSON.stringify(Array.from(document.querySelectorAll('input,select,textarea,button')).map(el=>({
  tag:el.tagName, type:el.type, name:el.name, id:el.id,
  placeholder:el.placeholder,
  selector: el.id ? '#'+el.id : (el.name ? el.tagName.toLowerCase()+"[name='"+el.name+"']" : '')
})))
```

### Step 2a: Use CSS selectors with fill/click
```
fill target='input[name=username]' value='admin'
fill target='input[name=password]' value='password123'
click target='button[type=submit]'
```

### Step 2b: OR do everything in ONE eval call (most efficient)
```
eval js:
(function(){
  var u = document.querySelector("input[name='username']");
  var p = document.querySelector("input[name='password']");
  var btn = document.querySelector("button[type='submit']");
  if(!u||!p||!btn) return JSON.stringify({error:"fields not found"});
  u.value="admin"; u.dispatchEvent(new Event("input",{bubbles:true}));
  p.value="pass123"; p.dispatchEvent(new Event("input",{bubbles:true}));
  btn.click();
  return JSON.stringify({status:"submitted"});
})()
```

### Step 3: Verify result
```
eval js: JSON.stringify({url:location.href, title:document.title, body:document.body.innerText.slice(0,500)})
```

> **CRITICAL**: If snapshot returns 0 refs, DO NOT retry snapshot. It will return 0 refs again.
> Switch to JavaScript strategy immediately.

---

## 2.6 SPA Pages Timing Issue (Vue/React/Angular)

SPA pages (identified by `#/` in URL) load a minimal HTML shell first, then JavaScript frameworks
render content **asynchronously**. This causes a timing gap:

- `Navigate()` completes when the base HTML `load` event fires
- But SPA framework components haven't mounted yet
- Both `snapshot` and `eval(querySelectorAll)` may return 0 elements

The `open` handler auto-detects hash routes and waits, but some pages need more time.

### If eval still returns 0 elements after open:

```
1. wait wait-type=ms wait-value=3000  (give SPA more time to render)
2. eval JS to discover elements (retry)
3. Or use a self-retrying JS pattern:
   (function poll(n){
     var els = document.querySelectorAll('input,button');
     if(els.length > 0 || n <= 0)
       return JSON.stringify(Array.from(els).map(e=>({tag:e.tagName,name:e.name,id:e.id,type:e.type})));
     return new Promise(r => setTimeout(() => r(poll(n-1)), 1000));
   })(5)
```

### Signs of SPA page:
- URL contains `#/` or `#!/`
- Page title is empty after load
- `readyState: "complete"` but `totalElements: 0`
- `hasVue`/`hasReact`/`hasAngular` flags in page info

> **RULE**: If eval returns 0 elements, WAIT then retry. Do NOT spin on snapshot or eval.

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

**Method A: JavaScript-First (RECOMMENDED)**

```
1. open url=https://target.com/login
2. eval js: JSON.stringify(Array.from(document.querySelectorAll('input,button')).map(el=>({
     tag:el.tagName, type:el.type, name:el.name, id:el.id,
     selector: el.id ? '#'+el.id : (el.name ? el.tagName.toLowerCase()+"[name='"+el.name+"']" : '')
   })))
   -> discover: input[name='username'], input[name='password'], button[type='submit']
3. fill target=input[name='username'] value=admin
4. fill target=input[name='password'] value=password123
5. click target=button[type='submit']
6. eval js: JSON.stringify({url:location.href, title:document.title, body:document.body.innerText.slice(0,300)})
   -> check if login succeeded
```

**Method B: Snapshot + Refs (only if snapshot returns refs)**

```
1. open url=https://target.com/login
   -> snapshot shows: textbox "Username" @e1, textbox "Password" @e2, button "Login" @e3
2. fill target=@e1 value=admin
3. fill target=@e2 value=password123
4. click target=@e3
5. snapshot -> check if login succeeded
```

> If snapshot in step 1 returns 0 refs, DO NOT retry. Switch to Method A immediately.

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
2. **Refs or CSS selectors**: Both `@eN` refs and CSS selectors (e.g. `input[name=username]`, `#submit-btn`) work as targets for `fill` and `click`. Use refs when snapshot provides them; use CSS selectors when discovered via `eval`.
3. **Set reasonable timeouts**: Use `timeout=10` for fast pages, `timeout=30` for slow ones.
4. **Wait for dynamic content**: Use `wait` before interacting with elements that load asynchronously.
5. **Close when done**: Always call `close` to release browser resources and avoid leaked processes.
6. **Check page state**: Use `get subop=title` or `get subop=url` to verify you are on the expected page before interacting.
7. **Handle errors gracefully**: If a click or fill fails, re-snapshot and inspect the page state. If snapshot returns 0 refs, switch to JavaScript strategy.
8. **Session persistence**: The browser instance persists across tool calls using the same session ID. Default session is `ai-browser`.
9. **When snapshot returns 0 refs**: This is common on SPA/React/Vue pages. **DO NOT retry snapshot** -- it will return 0 refs again. Instead:
   - Use `eval` with JS to discover form fields: `document.querySelectorAll('input,button')`
   - Use the CSS selectors from JS results directly with `fill` and `click`
   - Or perform the entire interaction (fill + submit) in a single `eval` call
   - Common causes: SPA frameworks, Shadow DOM, iframes, dynamically loaded content
10. **Prefer JavaScript for login/form pages**: A single `eval` call that fills form fields and clicks submit is more efficient and reliable than multiple separate fill/click calls, especially on dynamic pages.
11. **SPA timing: eval returns 0 elements too**: If `eval` also returns 0 elements (e.g. `totalElements: 0`), the SPA hasn't finished rendering yet. Use `wait wait-type=ms wait-value=3000` then retry eval. Do NOT spin -- wait first, then retry ONCE. If still 0, the page may use iframes or Shadow DOM.

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
