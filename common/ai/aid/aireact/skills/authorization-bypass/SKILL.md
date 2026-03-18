---
name: authorization-bypass
description: >
  Web 应用越权漏洞测试技能。覆盖水平越权(IDOR)、垂直越权(权限提升)、业务逻辑绕过
  三大类测试场景。提供基于 HTTP 请求篡改的系统化测试方法论，包括参数替换、Cookie/Token
  交换、角色 ID 篡改、隐藏字段操控、HTTP 方法变换、路径遍历、请求头伪造等具体技术。
  每种技术都映射到可直接调用的工具(do_http_request, send_http_request_packet, use_browser)，
  确保 AI 可以自动化执行越权测试。参考 OWASP WSTG-ATHZ-02/03/04 和 OWASP Top 10 A01。
---

# 越权漏洞测试技能 (Authorization Bypass Testing)

本技能指导 AI 系统化测试 Web 应用中的越权漏洞，覆盖水平越权 (Horizontal Privilege Escalation)、
垂直越权 (Vertical Privilege Escalation) 和业务逻辑绕过 (Business Logic Bypass) 三大类场景。

OWASP 参考: WSTG-ATHZ-02 (Authorization Schema Bypass), WSTG-ATHZ-03 (Privilege Escalation),
WSTG-ATHZ-04 (IDOR), OWASP Top 10 2021 A01 (Broken Access Control)

---

## 1. 核心概念

### 1.1 水平越权 (Horizontal Privilege Escalation / IDOR)

同一权限等级的用户 A 访问到用户 B 的数据或资源。

典型场景:
- `/api/user/123/profile` 改为 `/api/user/124/profile` 可查看其他用户资料
- `order_id=7001` 改为 `order_id=7002` 可查看他人订单
- 修改请求体中的 `user_id` 字段访问他人数据

### 1.2 垂直越权 (Vertical Privilege Escalation)

低权限用户执行了高权限用户才能执行的操作。

典型场景:
- 普通用户直接请求 `/admin/addUser` 创建管理员账户
- 修改请求中的 `role=user` 为 `role=admin` 实现自我提权
- 绕过前端按钮隐藏直接请求管理接口

### 1.3 业务逻辑绕过 (Business Logic Bypass)

利用业务流程缺陷绕过正常的权限校验。

典型场景:
- 跳过支付步骤直接请求订单完成接口
- 修改价格参数实现低价购买
- 利用优惠券重复使用、负数金额等逻辑缺陷

---

## 2. 前置准备: 双账户策略

越权测试的核心前提是准备至少两个不同身份的账户:

```
账户 A (受害者): 拥有目标资源的合法用户
  - 记录其 Session Cookie / JWT Token / API Key
  - 记录其用户 ID、订单 ID 等资源标识符

账户 B (攻击者): 尝试越权访问的用户
  - 同权限级别 (用于水平越权测试)
  - 低权限级别 (用于垂直越权测试)
  - 记录其 Session Cookie / JWT Token / API Key
```

关键操作: 在两个浏览器或使用不同 Session 分别登录两个账户，
收集各自的认证凭据和资源标识符。

---

## 3. 水平越权 (IDOR) 测试方法

### 3.1 URL 路径参数替换

最常见的 IDOR 场景: 替换 URL 中的资源 ID。

```
原始请求 (用户 A 的合法请求):
  GET /api/users/123/profile HTTP/1.1
  Cookie: session=USER_A_SESSION

攻击请求 (用户 B 尝试访问用户 A 的数据):
  GET /api/users/123/profile HTTP/1.1
  Cookie: session=USER_B_SESSION

判定: 如果用户 B 能看到用户 A 的 profile 数据，则存在水平越权。
```

工具执行:
```
Step 1: 用账户 A 的凭据发送正常请求，记录响应作为基线
  工具: do_http_request
  参数: url=/api/users/123/profile, Cookie=USER_A_SESSION

Step 2: 用账户 B 的凭据发送相同请求
  工具: do_http_request
  参数: url=/api/users/123/profile, Cookie=USER_B_SESSION

Step 3: 对比两次响应:
  - 响应相同且包含用户 A 的数据 -> 确认 IDOR
  - 返回 403/401 或无数据 -> 该端点有权限控制
```

### 3.2 查询参数替换

```
原始: GET /orders?order_id=7001  (用户 A 的订单)
攻击: GET /orders?order_id=7002  (用户 B 尝试访问其他订单)

原始: GET /invoice/download?id=1042
攻击: GET /invoice/download?id=1043
```

### 3.3 请求体参数替换

```
原始请求:
  POST /api/settings HTTP/1.1
  Cookie: session=USER_B_SESSION
  Content-Type: application/json

  {"user_id": "456", "email": "new@example.com"}

攻击请求 (替换 user_id):
  POST /api/settings HTTP/1.1
  Cookie: session=USER_B_SESSION
  Content-Type: application/json

  {"user_id": "123", "email": "attacker@example.com"}

判定: 如果用户 B 能修改用户 A(id=123) 的邮箱，则存在 IDOR。
```

### 3.4 ID 猜测与遍历技术

当 ID 不是简单递增整数时:

| ID 类型 | 猜测方法 |
|---------|---------|
| 递增整数 (123, 124, 125) | 直接 +1/-1 遍历 |
| UUID | 无法猜测，需从其他接口泄露获取 |
| Base64 编码 ID | 解码 -> 修改 -> 重新编码 |
| 哈希 ID (MD5/SHA) | 分析多个合法 ID 的模式，可能是可预测输入的哈希 |
| 时间戳型 | 枚举相近时间戳 |
| 自定义编码 | 对比多个合法值找规律 |

工具执行:
```
对于 Base64 编码的 ID:
  Step 1: decode base64 原始 ID
    工具: auto_decode 或 decode(base64, 原始值)
  Step 2: 修改解码后的值 (如数字+1)
  Step 3: encode base64 新值
    工具: encode(base64, 新值)
  Step 4: 用新值替换原始参数发送请求
    工具: do_http_request
```

### 3.5 HTTP 方法变换

有些应用对不同 HTTP 方法的权限检查不一致:

```
GET /api/users/123 -> 403 Forbidden (有权限检查)
PUT /api/users/123 -> 200 OK (忘记检查)
PATCH /api/users/123 -> 200 OK (忘记检查)
DELETE /api/users/123 -> 200 OK (忘记检查)
```

工具执行: 对同一端点依次尝试 GET, POST, PUT, PATCH, DELETE 方法。

### 3.6 批量 IDOR 检测 (Bulk ID Enumeration)

```
对于发现的 IDOR 端点，使用递增 ID 批量请求:

工具: do_http_request (循环执行)
URL 模式: /api/users/{ID}/profile
ID 范围: 1-100 (或根据已知 ID 推断范围)

分析: 统计成功响应 (200 OK) 的数量，确认数据泄露范围。
```

---

## 4. 垂直越权测试方法

### 4.1 直接请求管理端点

收集管理员可访问的 URL (通过爬虫、JS 文件分析、文档泄露等)，
然后使用普通用户凭据直接请求:

```
管理员端点列表 (从爬虫/JS 中发现):
  /admin/dashboard
  /admin/users
  /admin/settings
  /api/admin/create-user
  /api/admin/delete-user
  /api/admin/export-data

测试: 用普通用户 Cookie 逐个请求上述端点

工具: do_http_request
```

### 4.2 角色参数篡改

```
正常注册请求:
  POST /api/register HTTP/1.1
  Content-Type: application/json

  {"username": "newuser", "password": "pass123", "role": "user"}

攻击请求 (修改 role):
  POST /api/register HTTP/1.1
  Content-Type: application/json

  {"username": "newuser", "password": "pass123", "role": "admin"}
```

常见角色参数名:
- `role`, `role_id`, `user_role`, `group`, `group_id`
- `is_admin`, `isAdmin`, `admin`, `privilege`, `level`
- `type`, `user_type`, `account_type`

### 4.3 隐藏字段操控

前端可能隐藏了权限相关字段，但后端仍然处理:

```
原始表单 (前端隐藏了 role 字段):
  <input type="hidden" name="role" value="user">

攻击: 修改 hidden 字段值为 admin/superadmin，
      或在请求中添加额外的权限参数。

工具: send_http_request_packet (精确控制请求内容)
```

### 4.4 请求头伪造绕过

某些应用信任特定请求头进行权限判断:

```
尝试添加以下请求头绕过管理员限制:

X-Original-URL: /admin/dashboard
X-Rewrite-URL: /admin/dashboard
X-Forwarded-For: 127.0.0.1
X-Client-IP: 127.0.0.1
X-Remote-Addr: 192.168.1.1
X-Originating-IP: 10.0.0.1

工具: send_http_request_packet (自定义请求头)

检测方法:
  Step 1: 正常请求 GET / (记录响应)
  Step 2: 添加 X-Original-URL: /nonexistent (如果返回 404 说明支持该头)
  Step 3: 利用支持的头绕过访问控制
```

### 4.5 Cookie/Token 篡改

```
分析 Cookie 或 JWT Token 中的权限标识:

JWT Token 示例:
  Header: {"alg": "HS256", "typ": "JWT"}
  Payload: {"sub": "1234", "name": "user", "role": "user", "iat": 1516239022}

攻击: 修改 role 为 admin，如果签名校验薄弱 (如 alg=none) 可能绕过。

Cookie 示例:
  Cookie: role=dXNlcg==  (base64 of "user")
  攻击: role=YWRtaW4=  (base64 of "admin")

工具:
  decode(base64, "dXNlcg==") -> "user"
  encode(base64, "admin") -> "YWRtaW4="
```

---

## 5. 业务逻辑绕过测试方法

### 5.1 流程跳步 (Workflow Bypass)

```
正常购买流程:
  Step 1: 添加商品 -> POST /cart/add
  Step 2: 确认订单 -> POST /order/confirm
  Step 3: 支付 -> POST /payment/process
  Step 4: 完成 -> GET /order/complete

攻击: 跳过 Step 3 直接请求 Step 4
  工具: do_http_request
  请求: GET /order/complete?order_id=xxx
  判定: 如果能直接到达完成页面且订单状态变为已完成，则存在逻辑绕过。
```

### 5.2 参数篡改 (Price/Amount Tampering)

```
原始请求:
  POST /api/order HTTP/1.1
  {"product_id": "P001", "quantity": 1, "price": 9999}

攻击请求:
  POST /api/order HTTP/1.1
  {"product_id": "P001", "quantity": 1, "price": 1}
  或
  {"product_id": "P001", "quantity": -1, "price": 9999}

判定: 服务端是否信任客户端提交的价格/数量。
```

### 5.3 重复操作利用

```
优惠券重复使用:
  POST /api/coupon/apply
  {"coupon_code": "DISCOUNT50", "order_id": "O001"}

  快速重复发送多次 -> 是否重复扣减?

积分/余额竞态条件:
  同时发送多个提现/转账请求 -> 余额是否可能变负?

工具: do_http_request (快速连续发送)
```

### 5.4 API 版本绕过

```
当前版本有权限校验:
  GET /api/v2/admin/users -> 403 Forbidden

尝试旧版本:
  GET /api/v1/admin/users -> 200 OK (旧版本可能没有修复)
  GET /api/admin/users -> 200 OK (不带版本号)
```

---

## 6. 系统化测试执行流程

### 6.1 Phase 1: 信息收集

```
Step 1: 爬取目标应用
  工具: simple_crawler
  目的: 收集所有 URL、API 端点、表单、参数

Step 2: 识别关键端点
  工具: grep
  搜索模式: 含 id, user_id, order_id, role, admin 等参数的 URL
  搜索模式: /admin/, /api/, /user/, /account/ 路径

Step 3: 登录两个账户，收集凭据
  工具: use_browser (如果是 Web 表单登录)
  工具: do_http_request (如果是 API 登录)
  记录: 两个账户的 Cookie / Token / 资源 ID
```

### 6.2 Phase 2: 水平越权测试

```
对每个含资源 ID 的端点:

Step 1: 用账户 A 凭据发送正常请求 (基线)
  工具: do_http_request

Step 2: 用账户 B 凭据访问账户 A 的资源 (IDOR 测试)
  工具: do_http_request
  修改: 保持 URL 中的资源 ID 为账户 A 的值，
        替换 Cookie/Token 为账户 B 的

Step 3: 对比响应
  如果账户 B 能看到账户 A 的数据 -> IDOR 确认
  记录: write_file -> results/idor-<endpoint>.md
  报告: cybersecurity-risk (如果确认)
```

### 6.3 Phase 3: 垂直越权测试

```
对每个管理员/高权限端点:

Step 1: 用管理员凭据发送请求 (基线)
  工具: do_http_request

Step 2: 用普通用户凭据发送相同请求
  工具: do_http_request

Step 3: 对比响应
  如果普通用户能执行管理操作 -> 垂直越权确认
  记录: write_file -> results/priv-esc-<endpoint>.md
  报告: cybersecurity-risk (如果确认)
```

### 6.4 Phase 4: 业务逻辑测试

```
对关键业务流程:

Step 1: 完整执行正常流程，记录每一步的请求和响应
Step 2: 尝试跳步、参数篡改、重复操作
Step 3: 分析服务端是否正确校验
记录: write_file -> results/logic-<flow>.md
```

---

## 7. 关键 Payload 速查

### 7.1 IDOR 参数替换

| 原始值 | 测试值 | 说明 |
|--------|--------|------|
| `id=123` | `id=124`, `id=122`, `id=1` | 递增/递减/最小值 |
| `user_id=abc` | `user_id=def` | 其他用户的 ID |
| `uuid=xxx-yyy` | 从其他接口获取的 UUID | UUID 类 |
| `id=MTIz` | `id=MTI0` (base64 of 124) | Base64 编码 |
| `file=report_123.pdf` | `file=report_124.pdf` | 文件名中的 ID |
| `email=a@x.com` | `email=b@x.com` | 邮箱作为标识 |

### 7.2 角色提升参数

| 参数 | 正常值 | 攻击值 |
|------|--------|--------|
| `role` | `user` | `admin`, `administrator`, `superadmin`, `root` |
| `role_id` | `1` | `0`, `2`, `999` |
| `is_admin` | `false` | `true`, `1`, `yes` |
| `group` | `users` | `admins`, `staff`, `operators` |
| `privilege` | `read` | `write`, `admin`, `all` |
| `level` | `1` | `0`, `10`, `99` |
| `type` | `normal` | `admin`, `vip`, `internal` |

### 7.3 HTTP 方法替换

```
当 GET 被拒绝时，依次尝试:
POST, PUT, PATCH, DELETE, OPTIONS, HEAD, TRACE

当 POST 被拒绝时，尝试:
PUT, PATCH, GET (带查询参数)
```

### 7.4 绕过请求头

```
X-Original-URL: /admin/panel
X-Rewrite-URL: /admin/panel
X-Forwarded-For: 127.0.0.1
X-Client-IP: 127.0.0.1
X-Remote-Addr: 127.0.0.1
X-Real-IP: 127.0.0.1
X-Originating-IP: 127.0.0.1
X-Forwarded-Host: localhost
```

---

## 8. 使用 do_http_request 执行越权测试示例

### 8.1 水平越权 (IDOR) 测试

```
Step 1: 发送基线请求 (用户 A 访问自己的数据)
  工具: do_http_request
  method: GET
  url: https://target.com/api/user/123/orders
  headers: Cookie: session=USER_A_TOKEN

Step 2: 用用户 B 的凭据访问用户 A 的数据
  工具: do_http_request
  method: GET
  url: https://target.com/api/user/123/orders
  headers: Cookie: session=USER_B_TOKEN

Step 3: 分析 -> 如果 Step 2 返回了用户 A 的订单数据，确认 IDOR
```

### 8.2 垂直越权测试

```
Step 1: 用管理员凭据调用管理接口 (基线)
  工具: do_http_request
  method: POST
  url: https://target.com/api/admin/create-user
  headers: Cookie: session=ADMIN_TOKEN
  body: {"username": "test", "role": "user"}

Step 2: 用普通用户凭据调用相同接口
  工具: do_http_request
  method: POST
  url: https://target.com/api/admin/create-user
  headers: Cookie: session=USER_TOKEN
  body: {"username": "test2", "role": "user"}

Step 3: 分析 -> 如果 Step 2 成功创建用户，确认垂直越权
```

### 8.3 使用 send_http_request_packet 精确测试

当需要精确控制请求内容 (自定义头部、特殊编码等) 时:

```
工具: send_http_request_packet
原始请求包:

GET /admin/dashboard HTTP/1.1
Host: target.com
Cookie: session=USER_TOKEN
X-Original-URL: /admin/dashboard
X-Forwarded-For: 127.0.0.1
```

### 8.4 使用 use_browser 测试 (Web 表单场景)

当越权需要通过 Web UI 操作时 (如修改表单隐藏字段):

```
Step 1: 用浏览器登录用户 B
  工具: use_browser op=open url=https://target.com/login
  工具: use_browser op=eval js="填写登录表单并提交"

Step 2: 直接访问用户 A 的页面
  工具: use_browser op=eval js="location.href='/user/123/profile'"

Step 3: 检查是否能看到用户 A 的数据
  工具: use_browser op=eval js="document.body.innerText"
```

---

## 9. 漏洞严重程度判定

| 漏洞类型 | 影响 | 严重程度 |
|---------|------|---------|
| IDOR 读取敏感数据 (PII, 金融) | 大量用户数据泄露 | High |
| IDOR 读取非敏感数据 | 信息泄露但影响有限 | Medium |
| IDOR 修改/删除他人数据 | 数据完整性破坏 | High-Critical |
| 垂直越权到管理员 | 完全接管应用 | Critical |
| 垂直越权到部分高权限功能 | 越权操作 | High |
| 业务逻辑绕过 (支付/价格) | 经济损失 | High-Critical |
| 业务逻辑绕过 (流程跳步) | 业务流程破坏 | Medium-High |

---

## 10. 常见场景速查

### 场景 1: "测试这个 API 的越权问题"

```
1. 使用两个不同账户登录，获取各自的认证 Token
2. 爬取 API 端点列表 (simple_crawler 或 read_file)
3. 识别含资源 ID 的端点 (grep 搜索 id, user_id 等)
4. 对每个端点: 用用户 B 的 Token 请求用户 A 的资源
5. 分析响应，记录可越权的端点
6. 用用户 B 的 Token 请求管理员端点
7. 汇总报告
```

### 场景 2: "测试这个登录后的页面有没有水平越权"

```
1. 用 use_browser 登录账户 A，记录关键数据的 URL 和参数
2. 用 use_browser 登录账户 B (新 session)
3. 用账户 B 的浏览器 session 直接访问账户 A 的数据 URL
4. 检查是否能看到账户 A 的数据
```

### 场景 3: "检查管理后台是否有垂直越权"

```
1. 用管理员账户爬取管理后台的所有 URL
2. 记录管理接口列表
3. 用普通用户的 Cookie 逐个请求管理接口
4. 分析哪些接口缺少权限校验
```

---

## 11. 注意事项

1. **不要盲目遍历**: 越权测试需要有具体的目标端点，不是对所有 URL 暴力测试。
2. **保留证据**: 每次测试都要记录请求和响应，作为漏洞证明。
3. **最小影响**: 测试修改/删除操作时优先用测试账户的数据，避免破坏其他用户数据。
4. **Token 过期**: 注意 Session/JWT 的有效期，过期后需要重新获取。
5. **速率限制**: 批量遍历 ID 时注意不要触发 WAF 或速率限制。
6. **结合其他漏洞**: IDOR 常与信息泄露、CSRF 等漏洞配合利用。
