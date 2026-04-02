# HTTP Fuzz Test Loop 设计规划

## 1. 背景

`loop_http_fuzztest` 当前已经具备一个最小可用闭环：

- 通过 `set_http_request` 设置原始 HTTP 请求
- 通过 `fuzz_method / fuzz_path / fuzz_header / fuzz_get_params / fuzz_body / fuzz_cookie` 对请求不同位置做变异
- 发送请求并基于状态码、响应体长度、响应体前 200 字节做粗粒度 diff
- 在 init 阶段基于用户意图检索安全知识库

这套实现可以支持简单的单点 payload 测试，但距离“AI 特化的手工发包渗透测试 loop”还有明显差距，主要缺口如下：

- 缺少对请求面的自动识别，AI 不知道当前请求有哪些可测点
- 缺少测试计划、覆盖记录、发现列表等显式状态
- 动作按“请求位置”切分，缺少按“渗透测试场景”组织的能力
- 还没有把 `mutate.FuzzHTTPRequest` 已有的 `JSON / Base64 / Base64+JSON / XML / multipart / upload` 能力暴露出来
- 缺少对弱口令、ID 枚举、敏感信息泄漏、编码绕过等常见手工场景的专门流程
- 缺少对 Yak fuzztag 的系统引导，AI 还不会稳定利用 `{{int(...)}}`、`{{array(...)}}`、`{{base64(...)}}`、`{{urlenc(...)}}` 等标签生成高价值测试样本
- diff 分析过于粗糙，没有时间、关键字、头部、重定向、反射、错误栈等更有价值的判定信号

本规划的目标，是把该 loop 设计成一个适合“手工 HTTP 发包安全测试”的 AI 回路，而不是通用扫描器。

本规划额外采用一个重要前提：

- 当前目录下已有代码只作为现状调研样本
- 后续实现允许直接重写 loop 结构、prompt、action 与状态模型
- 不以兼容当前 action 名称、状态字段或控制流程为目标

## 2. 设计目标

### 2.1 目标能力

该 loop 需要覆盖以下典型手工发包场景：

- 通用漏洞测试：SQL 注入、XSS、SSTI、命令注入、路径遍历、SSRF、模板/表达式注入、Header 注入、认证绕过
- 弱密码测试：登录表单、JSON 登录接口、Basic/Bearer/Cookie 认证字段
- ID 枚举与爆破：整数 ID、订单号、用户号、短 token、邀请码、对象编号
- 敏感信息泄漏检查：调试接口、备份路径、Swagger、配置文件、报错栈、历史接口
- 编码/绕过测试：URL 编码、双重 URL 编码、Base64、Unicode、HTML 实体、HEX、大小写混淆、注释绕过、空白符绕过
- 特殊结构测试：JSON Path、Base64 包裹参数、Cookie/Body 中嵌套 JSON、XML、multipart、上传文件名/文件内容

### 2.2 非目标

当前 loop 不应承担以下职责：

- 大规模资产扫描
- 自动利用链编排
- 跨站点多步骤业务流编排
- 复杂浏览器端验证
- 长时间高并发爆破

这些能力应由其他模块或后续更高层工作流负责。

## 3. 核心设计原则

### 3.1 从“位置驱动”升级到“场景驱动 + 位置驱动”

现有 action 是“改哪里”，后续应同时支持“为什么改”。

理想状态下，AI 既能说：

- 我现在要 fuzz `id` 这个 GET 参数

也能说：

- 我现在要做 ID 枚举测试
- 我现在要做登录弱口令测试
- 我现在要做编码绕过测试

因此，动作层要分为两层：

- 底层能力动作：精确操作某个参数或请求结构
- 高层场景动作：围绕某个渗透测试目标组织样本、控制节奏、判断结果

### 3.2 参数发现先于参数测试

AI 必须先获得“可测试面”的结构化视图，再进入 fuzz。底层 `mutate.FuzzHTTPRequest` 已经能识别：

- GET 参数
- POST 表单参数
- JSON Body 参数
- XML 参数
- Cookie 参数
- Header 参数
- Path
- Method
- Base64 参数
- Base64 包裹 JSON 参数

因此 loop 启动后应尽快生成参数清单，而不是只保存原始请求字符串。

### 3.3 把 Yak fuzztag 视为一等公民

凡是满足以下条件，应优先引导 AI 使用 fuzztag，而不是手写长 payload 列表：

- 数值区间枚举
- 用户名/密码字典
- 多编码绕过
- 大小写混淆
- 重复发送
- Base64 / URL / HTML / Unicode / HEX 编码
- 组合型 payload 模板

### 3.4 以“人工可理解”的证据驱动结论

loop 的输出不能只说“可能有问题”，而要能保留：

- 测试位置
- 使用的 payload 或 fuzztag 模板
- 基线响应特征
- 异常响应特征
- 判定理由
- 后续建议

### 3.5 采用绿地重构，而不是兼容式修补

既然当前代码不要求保留，后续设计应优先追求：

- 场景模型正确
- action 语义清晰
- loop 状态简洁且可扩展
- prompt 与工具能力一一对应

不应为了兼容已有实现而保留不理想的边界，例如：

- 仅按 `method/path/header/get/body/cookie` 切分 action
- 只保存 `original/last/diff` 这种过薄状态
- 把高级场景强行塞进旧 action 参数

## 4. 现状能力与可复用基础

当前目录和底层库已经提供了后续设计的基础：

- `mutate.FuzzHTTPRequest.GetAllParams()` 可抽取参数面
- `FuzzHTTPRequestParam` 已区分 `GET / POST / JSON / XML / Cookie / Header / Path / Method / Body`
- 已支持 `FuzzGetBase64Params / FuzzPostBase64Params / FuzzCookieBase64`
- 已支持 `FuzzGetJsonPathParams / FuzzPostJsonPathParams`
- 已支持 `FuzzGetBase64JsonPath / FuzzPostBase64JsonPath / FuzzCookieBase64JsonPath`
- 已支持 `FuzzUploadFile / FuzzUploadFileName / FuzzUploadKVPair / FuzzFormEncoded`
- Yak fuzztag 已提供 `int / array / randstr / repeat / base64 / urlenc / doubleurlenc / htmlenc / htmlhexenc / unicode / hex / randomupper / fuzz:username / fuzz:password`

这说明后续设计不应重新造底层能力，而应重点解决：

- 如何把这些能力暴露成对 AI 友好的 action
- 如何让 AI 形成稳定、可控的测试策略
- 如何让 loop 记住覆盖情况和发现情况

## 5. 场景覆盖矩阵

### 5.1 通用漏洞测试

需要覆盖的输入位置：

- GET 参数
- POST 表单
- JSON 字段
- XML 节点
- Cookie
- Header
- Path
- Raw Body

需要覆盖的漏洞类别：

- SQL 注入
- XSS
- SSTI
- 命令注入
- 路径遍历 / 文件读取
- SSRF / URL 注入
- Header 注入 / CRLF
- 认证绕过
- 业务逻辑绕过

### 5.2 弱密码测试

重点模式：

- 用户名 + 密码双参数
- 只有密码参数，用户名固定
- JSON 登录接口
- Basic Auth
- Authorization Bearer 伪造或空值旁路
- Session/Cookie 结合登录态测试

### 5.3 ID 枚举 / 爆破 / IDOR 辅助

重点模式：

- GET `id / uid / userId / orderId / docId`
- Path 中的数值片段
- JSON 中的对象 ID
- Base64 包裹 ID
- 连续整数、零填充编号、短 token、枚举值切换

### 5.4 敏感信息泄漏

重点模式：

- 路径扩展探测：`/swagger`、`/swagger.json`、`/.git/config`、`/.env`、`/backup.zip`
- Header 探测：`X-Forwarded-For`、调试头、源站头
- 触发报错：非法类型、非法编码、闭合符、边界值
- 数据越权：枚举后响应数据差异

### 5.5 编码绕过

重点模式：

- URL 编码
- 双重 URL 编码
- Base64
- Unicode
- HTML 实体
- HEX
- 大小写混淆
- 注释绕过
- 空白符绕过

## 6. 目标 Loop 工作流

建议把 loop 设计成六个阶段。

### 6.1 阶段一：请求归一化与请求面分析

进入 loop 后先完成以下动作：

- 保存原始请求
- 解析协议、方法、路径、查询串、Content-Type、Cookie、认证头
- 识别参数清单和参数类型
- 识别候选高价值字段
- 建立基线请求与基线响应

高价值字段识别规则应包含：

- 常见注入字段：`q, search, keyword, name, title, content`
- 常见 ID 字段：`id, uid, userId, orderId, docId`
- 常见认证字段：`username, user, account, password, passwd, token, auth, session`
- 常见 URL 字段：`url, redirect, callback, return, next, target`
- 常见文件字段：`file, path, filename, dir`

### 6.2 阶段二：场景识别与测试计划生成

AI 基于：

- 用户输入
- 请求结构
- 参数命名
- Content-Type
- 基线响应

生成结构化测试计划，至少包括：

- 本轮优先场景
- 对应参数目标
- 每个参数要跑的 payload/profile
- 预估的测试顺序
- 停止条件

测试计划不应一次性铺满所有组合，而要分批次执行。

### 6.3 阶段三：低噪声基线探测

先做低风险、低成本试探，以判断应用的解析与过滤特征：

- 引号闭合
- 特殊字符
- 长度变化
- 空值/空白值
- 类型错配
- HTTP 方法切换
- 非法路径片段
- 基础编码变体

该阶段的目标不是直接证明漏洞，而是建立“异常模式”。

### 6.4 阶段四：场景化深入测试

当某一位置出现高价值信号后，再升级到场景化测试：

- SQL 注入分为错误型、布尔型、时间型、联合型、绕过型
- XSS 分为 HTML 上下文、属性上下文、JS 上下文、URL 上下文
- 弱密码测试分为用户名字典、密码字典、用户名密码笛卡尔组合
- ID 枚举分为连续整数、零填充、字典值、Base64 编码枚举
- 敏感信息泄漏分为路径扩展、报错触发、越权数据差异

### 6.5 阶段五：结果聚类与证据沉淀

loop 不应只保存最后一个响应，而应维护：

- 异常结果列表
- 已确认发现列表
- 每个参数的覆盖历史
- 每种场景的命中信号

### 6.6 阶段六：完成判定

可结束条件应包括：

- 用户指定的目标场景已经覆盖
- 高价值参数均已完成基础探测
- 异常结果已被复测确认
- 继续测试的收益明显降低

## 7. 建议新增或重构的 Loop 状态

当前仅有 `original_request / last_request / last_response / diff_result / security_knowledge`，不足以支撑复杂测试。建议新增以下状态。

### 7.1 请求分析状态

- `request_profile`
- `parameter_inventory`
- `high_value_targets`
- `auth_profile`
- `content_profile`

### 7.2 测试执行状态

- `baseline_response`
- `baseline_fingerprint`
- `test_plan`
- `coverage_map`
- `active_scenario`
- `attempt_history`

### 7.3 结果状态

- `anomaly_candidates`
- `confirmed_findings`
- `interesting_responses`
- `next_recommended_actions`

## 8. Action 设计规划

建议直接采用全新的 action 模型，不以兼容当前 action 为目标。当前实现中的动作拆分方式只保留为参考，不应约束新设计。

### 8.1 基础动作

#### 8.1.1 `load_http_request`

作为新的入口动作，职责包括：

- 解析并保存参数清单
- 尝试自动建立基线响应
- 识别是否为 HTTPS、是否存在认证态、是否为 JSON/XML/multipart
- 生成后续测试所需的内部请求模型

#### 8.1.2 `inspect_request_surface`

新增。

用途：

- 输出结构化请求面
- 列出参数位置、参数名、值类型、是否为 JSONPath、是否为 Base64、是否疑似认证字段、是否疑似 ID 字段

这是后续所有 fuzz 的前置可见性动作。

#### 8.1.3 `mutate_target`

统一的底层变异动作，用于替代旧式按位置分裂的 action。

核心参数建议：

- `target_ref`
- `target_position`
- `mutation_mode`
- `payloads`
- `use_fuzztag`
- `encoding_policy`
- `disable_auto_encode`
- `reason`

`target_ref` 应能引用：

- 普通参数名
- JSONPath
- XML XPath
- Path
- Method
- Header
- Cookie
- Base64 参数内部路径

`mutation_mode` 建议支持：

- `replace`
- `append`
- `prefix`
- `suffix`
- `raw_replace`
- `jsonpath_replace`
- `base64_wrap`

#### 8.1.4 `execute_test_batch`

新增。

用途：

- 在同一目标上批量重放一组 payload/profile
- 支持限制发送数量
- 支持复测异常样本

#### 8.1.5 `commit_finding`

新增。

用途：

- 将当前异常结果转成“证据化结论”
- 避免 loop 只会不断继续 fuzz 而不会收敛

### 8.2 场景动作

#### 8.2.1 `run_generic_vuln_test`

新增。

面向：

- SQLi
- XSS
- SSTI
- CMDi
- Path Traversal
- SSRF
- CRLF

输入应包含：

- `scenario`
- `target_refs`
- `profile`
- `depth`

#### 8.2.2 `run_weak_password_test`

新增。

支持：

- 用户名参数、密码参数识别
- 用户名固定 / 密码固定 / 双字段组合
- 表单、JSON、Basic Auth、Cookie 登录
- 使用 `{{fuzz:username(...)}}` 与 `{{fuzz:password(...)}}`

#### 8.2.3 `run_identifier_enumeration`

新增。

支持：

- 连续整数爆破
- 零填充编号
- 短 token
- Base64 包裹 ID
- Path 段枚举

推荐结合：

- `{{int(1-100)}}`
- `{{int(1-100|4)}}`
- `{{array(admin|test|guest)}}`

#### 8.2.4 `run_sensitive_info_exposure_test`

新增。

支持：

- 敏感路径探测
- 备份文件探测
- Swagger/Actuator/调试路径
- 报错栈诱导
- 环境信息、版本信息、绝对路径、账号字段识别

#### 8.2.5 `run_encoding_bypass_test`

新增。

支持：

- 以同一原始 payload 生成多种编码变体
- 组合 `urlenc / doubleurlenc / base64 / unicode / htmlenc / hex / randomupper`
- 适用于 SQLi、Traversal、XSS、Header 注入

### 8.3 重构策略

既然不要求兼容旧代码，建议直接按新模型重建：

- 旧 action 不作为设计输入，只作为调研参考
- 新 loop 以“请求分析 -> 测试计划 -> 场景执行 -> finding 收敛”为主干
- prompt、状态字段、action schema 一次性按新模型统一命名
- 若需要迁移，可在最后做最薄的一层适配，而不是让新设计受旧接口牵制

## 9. Yak fuzztag 集成策略

本 loop 必须显式教育 AI 何时使用 fuzztag，避免 AI 每次都人工枚举字符串。

### 9.1 推荐优先使用 fuzztag 的场景

#### 9.1.1 ID 枚举

- `{{int(1-10)}}`
- `{{int(1-100|4)}}`
- `{{array(1001|1002|1003)}}`

#### 9.1.2 弱密码测试

- `{{fuzz:username(admin)}}`
- `{{fuzz:password(admin)}}`
- `{{array(admin|root|test)}}`

#### 9.1.3 编码绕过

- `{{urlenc(...)}}`
- `{{doubleurlenc(...)}}`
- `{{base64(...)}}`
- `{{unicode(...)}}`
- `{{htmlenc(...)}}`
- `{{htmlhexenc(...)}}`
- `{{hex(...)}}`
- `{{randomupper(...)}}`

#### 9.1.4 批量重放与压测式重复

- `{{repeat(3)}}`
- `{{randstr(8)}}`

### 9.2 prompt 中必须写清楚的规则

- 需要范围、字典、编码、随机化时优先使用 fuzztag
- 需要对同一原始 payload 派生多种编码变体时，优先保留“语义原文 + 编码标签”的结构
- 当目标字段本身就是 Base64 数据时，优先使用 Base64 专用 mutation，而不是手工把整个字段当普通字符串替换
- 当目标字段是 JSON 内嵌值时，优先使用 JSONPath 定位，而不是粗暴替换整个 body

### 9.3 本 loop 推荐内置的 payload profile

建议在文档或 prompt 中维护 profile 概念，而不是散乱写 payload。

建议 profile 包括：

- `sqli_basic`
- `sqli_boolean`
- `sqli_error`
- `sqli_time`
- `sqli_bypass_encoded`
- `xss_html`
- `xss_attr`
- `xss_js`
- `cmdi_basic`
- `traversal_basic`
- `traversal_encoded`
- `ssti_basic`
- `weakpass_basic`
- `id_enum_numeric`
- `id_enum_zero_padded`
- `debug_leak_probe`

profile 内部样本优先由 fuzztag 模板表达。

## 10. 响应分析与判定设计

当前 diff 只看：

- 状态码
- body 长度
- body 前 200 字符

这不足以支撑渗透测试结论。后续需要把分析升级为“响应指纹 + 异常信号”。

### 10.1 基线响应指纹

建议基线至少包含：

- 状态码
- Location
- Content-Type
- Content-Length
- Header 差异摘要
- 关键词命中
- 页面标题
- 错误栈特征
- 响应时间
- 是否反射 payload

### 10.2 异常信号

建议统一抽象以下信号：

- `status_changed`
- `length_delta_large`
- `time_delay_detected`
- `error_signature_detected`
- `payload_reflected`
- `unescaped_reflection`
- `redirect_changed`
- `auth_state_changed`
- `record_count_changed`
- `sensitive_keyword_detected`

### 10.3 场景化判定规则

例如：

- SQLi：报错关键字、真假条件响应差异、明显延迟、结果集变化
- XSS：原样反射、未转义标签、事件处理器回显
- 弱密码：登录成功标记、重定向变化、Set-Cookie 变化、用户中心信息出现
- ID 枚举：不同 ID 返回不同对象信息、数量变化、用户标识变化
- 敏感信息泄漏：路径、密钥、环境变量、堆栈、版本号、接口描述文档命中

## 11. 提示词与知识组织规划

当前 prompt 只覆盖基础 fuzz 指令，后续应补齐以下内容。

### 11.1 `persistent_instruction.txt`

需要新增：

- 参数发现优先
- 测试计划生成
- 高价值字段优先级
- fuzztag 优先策略
- 各场景推荐测试顺序
- 何时复测、何时结束

### 11.2 `reactive_data.txt`

需要新增：

- 参数清单
- 已测覆盖面
- 当前活跃场景
- 已发现异常
- 基线响应指纹
- 本轮剩余建议动作

### 11.3 guide 文档

建议增加以下 guide：

- `weak_password_guide.md`
- `idor_identifier_guide.md`
- `sensitive_info_leak_guide.md`
- `encoding_bypass_guide.md`

现有的 `sql_injection_guide.md` 与 `xss_injection_guide.md` 可以继续保留。

## 12. 推荐的优先级与实现阶段

为了避免一次性改动过大，建议分阶段落地。

### 12.1 第一阶段

先重建 loop 主骨架：

- 新入口动作与请求内部模型
- 请求面分析
- 参数清单生成
- 高价值目标识别
- 基线响应建立
- 覆盖状态记录

这是后续一切高级能力的基础。

### 12.2 第二阶段

补统一变异与执行能力：

- 通用 `mutate_target`
- `execute_test_batch`
- Base64 / JSONPath / XML / multipart / upload 定位
- 编码策略与 auto encode 控制

### 12.3 第三阶段

补场景动作：

- 通用漏洞测试
- 弱密码测试
- ID 枚举
- 敏感信息泄漏
- 编码绕过

### 12.4 第四阶段

补结论能力：

- 响应指纹
- 异常聚类
- 证据化 finding
- 完成判定与收敛策略

## 13. 建议的最终设计取向

这个 loop 的理想形态，不是“AI 随机往请求里塞 payload”，而是：

1. 先理解请求结构与业务意图
2. 再识别最值得测的参数和场景
3. 优先使用 Yak 原生 fuzztag 构造高质量样本
4. 基于响应差异逐步加深测试
5. 最后把异常沉淀成可解释的安全结论

如果按这个方向演进，`loop_http_fuzztest` 会从“参数 fuzz 演示版”升级为“适合手工 HTTP 发包渗透测试的 AI 专用 loop”。

## 14. 实现级模块拆分

为避免后续实现耦合过深，建议按以下模块边界重建。

### 14.1 `loop_runtime`

职责：

- 管理 loop 生命周期
- 管理状态读写
- 调用 action
- 组织 prompt 渲染
- 处理完成判定与异常退出

### 14.2 `request_model`

职责：

- 将原始 HTTP 请求解析为统一内部模型
- 提供请求面枚举
- 提供 target 引用解析
- 提供位置到变异器的映射

### 14.3 `surface_analyzer`

职责：

- 提取参数清单
- 识别高价值字段
- 识别登录请求、查询请求、详情请求、上传请求
- 识别 Base64、JSON、XML、Path 片段、认证头

### 14.4 `scenario_planner`

职责：

- 根据用户目标和请求面生成测试计划
- 决定优先级
- 控制同类测试深度
- 决定是否升级到深入测试

### 14.5 `mutation_engine`

职责：

- 将 target + mutation spec 转换为可执行请求变体
- 支持普通替换、追加、前后缀、编码派生、JSONPath、Base64 内部变异、文件名/文件内容变异
- 接入 Yak fuzztag

### 14.6 `batch_executor`

职责：

- 批量执行请求
- 控制单批次数量
- 记录 payload 到响应的映射
- 为后续指纹分析提供结构化结果

### 14.7 `response_fingerprint`

职责：

- 生成基线指纹
- 生成单响应指纹
- 对比并产出异常信号

### 14.8 `finding_engine`

职责：

- 对异常结果聚类
- 做复测确认
- 形成 finding
- 生成下一步建议

### 14.9 `payload_profiles`

职责：

- 提供各类漏洞与场景的 profile
- 维护 fuzztag 模板
- 为 planner 和 action 提供统一 profile 名

## 15. 内部数据模型

建议在设计阶段就固定核心状态结构，避免后续出现字段漂移。

### 15.1 `request_profile`

建议结构：

```json
{
  "scheme": "https",
  "host": "example.com",
  "method": "POST",
  "path": "/api/user/login",
  "content_type": "application/json",
  "has_cookie": true,
  "has_authorization": false,
  "is_multipart": false,
  "is_json_body": true,
  "is_xml_body": false,
  "business_guess": "login",
  "risk_hints": ["auth", "json", "credential_input"]
}
```

### 15.2 `parameter_inventory`

建议结构：

```json
[
  {
    "target_ref": "json:$.username",
    "position": "json_body",
    "name": "username",
    "path": "$.username",
    "value_preview": "admin",
    "value_type": "string",
    "encoding": "plain",
    "high_value_tags": ["credential", "identifier_candidate"],
    "supported_mutations": ["replace", "prefix", "suffix"]
  },
  {
    "target_ref": "json:$.password",
    "position": "json_body",
    "name": "password",
    "path": "$.password",
    "value_preview": "***",
    "value_type": "string",
    "encoding": "plain",
    "high_value_tags": ["credential", "secret"],
    "supported_mutations": ["replace"]
  }
]
```

### 15.3 `test_plan`

建议结构：

```json
{
  "user_goal": "测试登录接口是否存在弱口令和认证绕过",
  "active_scenarios": [
    {
      "scenario": "weak_password",
      "priority": 10,
      "targets": ["json:$.username", "json:$.password"],
      "profiles": ["weakpass_basic"],
      "depth": "medium",
      "stop_when": ["auth_success_detected", "lockout_detected", "budget_exhausted"]
    },
    {
      "scenario": "auth_bypass",
      "priority": 7,
      "targets": ["method", "header:Authorization", "cookie:session"],
      "profiles": ["auth_bypass_basic"],
      "depth": "low",
      "stop_when": ["auth_state_changed", "budget_exhausted"]
    }
  ],
  "remaining_budget": {
    "max_requests": 80,
    "max_batches": 12
  }
}
```

### 15.4 `attempt_history`

建议结构：

```json
[
  {
    "batch_id": "b1",
    "scenario": "weak_password",
    "target_refs": ["json:$.username", "json:$.password"],
    "profile": "weakpass_basic",
    "request_count": 12,
    "anomaly_count": 2,
    "summary": "发现两组凭据返回长度和重定向不同"
  }
]
```

### 15.5 `anomaly_candidates`

建议结构：

```json
[
  {
    "candidate_id": "a1",
    "scenario": "sqli",
    "target_ref": "query:id",
    "payload": "1' AND SLEEP(5)--",
    "signals": ["time_delay_detected"],
    "confidence": "medium",
    "needs_retest": true
  }
]
```

### 15.6 `confirmed_findings`

建议结构：

```json
[
  {
    "finding_id": "f1",
    "category": "weak_password",
    "severity": "high",
    "target_refs": ["json:$.username", "json:$.password"],
    "evidence": [
      "admin/admin123 返回 302 /dashboard",
      "错误凭据返回 200 且无 Set-Cookie"
    ],
    "conclusion": "接口接受弱口令 admin/admin123 登录",
    "next_step": "检查是否存在 MFA、验证码或锁定策略"
  }
]
```

## 16. Target 引用规范

为了让 action 可稳定引用测试位置，建议引入统一 `target_ref` 语法。

### 16.1 基础格式

```text
method
path
path:block:2
header:Authorization
header:X-Forwarded-For
query:id
query_raw
cookie:session
body_raw
form:username
json:$.user.id
xml:/root/user/id
query_base64:data
query_base64_json:data:$.user.id
form_base64:token
form_base64_json:data:$.auth.uid
cookie_base64:rememberMe
cookie_base64_json:rememberMe:$.uid
upload_filename:file
upload_content:file
```

### 16.2 设计原则

- `target_ref` 必须可逆映射到唯一请求位置
- AI 输出时优先使用 `target_ref`，而不是模糊描述
- `target_ref` 解析失败时，action 应明确报错并要求重新选择

## 17. Action Schema 细化

这里给出建议的 action 规格。后续实现可以略调字段名，但语义不应漂移。

### 17.1 `load_http_request`

输入建议：

```json
{
  "@action": "load_http_request",
  "http_request": "RAW HTTP REQUEST",
  "is_https": true,
  "reason": "加载待测登录请求"
}
```

输出效果：

- 生成 `request_profile`
- 生成 `parameter_inventory`
- 自动发送一次或少量基线请求
- 写入 `baseline_response` 与 `baseline_fingerprint`

### 17.2 `inspect_request_surface`

输入建议：

```json
{
  "@action": "inspect_request_surface",
  "focus": ["credential", "identifier", "url_like", "file_like"],
  "reason": "识别高价值可测点"
}
```

输出效果：

- 对参数清单做摘要
- 标注高价值字段
- 为 planner 提供候选 target

### 17.3 `mutate_target`

输入建议：

```json
{
  "@action": "mutate_target",
  "target_ref": "query:id",
  "mutation_mode": "replace",
  "payloads": ["1", "1'", "1 OR 1=1", "{{urlenc(1' OR '1'='1)}}"],
  "use_fuzztag": true,
  "encoding_policy": "preserve",
  "disable_auto_encode": false,
  "reason": "对 id 做基础 SQL 注入探测"
}
```

字段要求：

- `payloads` 允许普通字符串和 fuzztag 模板混用
- `encoding_policy` 建议支持 `preserve / force_url / force_base64 / no_encode / inherit`
- `disable_auto_encode` 用于配合特殊绕过

### 17.4 `execute_test_batch`

输入建议：

```json
{
  "@action": "execute_test_batch",
  "scenario": "sqli",
  "target_refs": ["query:id"],
  "profile": "sqli_basic",
  "variant_source": "last_mutation",
  "max_requests": 12,
  "reason": "执行一批基础 SQL 注入 payload"
}
```

输出效果：

- 产生结构化测试结果
- 记录 request/response 对应关系
- 生成 anomalies 初判

### 17.5 `run_generic_vuln_test`

输入建议：

```json
{
  "@action": "run_generic_vuln_test",
  "scenario": "xss",
  "target_refs": ["query:q"],
  "profile": "xss_html",
  "depth": "medium",
  "prefer_fuzztag": true,
  "reason": "对搜索参数做 XSS 测试"
}
```

建议支持场景：

- `sqli`
- `xss`
- `ssti`
- `cmdi`
- `traversal`
- `ssrf`
- `crlf`
- `auth_bypass`

### 17.6 `run_weak_password_test`

输入建议：

```json
{
  "@action": "run_weak_password_test",
  "username_targets": ["json:$.username"],
  "password_targets": ["json:$.password"],
  "username_strategy": "dictionary",
  "password_strategy": "top_weak",
  "max_pairs": 30,
  "prefer_fuzztag": true,
  "reason": "测试登录接口弱口令"
}
```

### 17.7 `run_identifier_enumeration`

输入建议：

```json
{
  "@action": "run_identifier_enumeration",
  "target_ref": "path:block:3",
  "strategy": "numeric_range",
  "range_template": "{{int(1-50|4)}}",
  "max_requests": 20,
  "reason": "枚举四位零填充订单号"
}
```

### 17.8 `run_sensitive_info_exposure_test`

输入建议：

```json
{
  "@action": "run_sensitive_info_exposure_test",
  "mode": "path_probe",
  "path_profile": "debug_leak_probe",
  "max_requests": 15,
  "reason": "检查常见调试与备份路径"
}
```

### 17.9 `run_encoding_bypass_test`

输入建议：

```json
{
  "@action": "run_encoding_bypass_test",
  "target_ref": "query:file",
  "base_payload": "../etc/passwd",
  "encodings": ["url", "double_url", "unicode", "hex"],
  "mixed_case": false,
  "reason": "测试路径遍历编码绕过"
}
```

### 17.10 `commit_finding`

输入建议：

```json
{
  "@action": "commit_finding",
  "candidate_ids": ["a1", "a2"],
  "category": "sqli",
  "severity": "high",
  "reason": "时间型 payload 稳定触发 5 秒延迟"
}
```

输出效果：

- 将异常候选固化为 finding
- 允许 loop 进入收敛阶段

## 18. 响应指纹结构

建议统一使用结构化响应指纹，而不是散乱拼文本。

### 18.1 `response_fingerprint`

```json
{
  "status_code": 200,
  "content_type": "text/html",
  "content_length": 18234,
  "header_digest": {
    "location": "",
    "set_cookie_count": 1,
    "server_hint": "nginx"
  },
  "body_digest": {
    "title": "User Center",
    "preview": "<html><title>User Center</title>",
    "keyword_hits": ["welcome", "logout"],
    "error_signatures": [],
    "reflection_hits": ["admin"],
    "sensitive_hits": []
  },
  "timing": {
    "duration_ms": 523
  }
}
```

### 18.2 `diff_signals`

```json
{
  "signals": [
    "status_changed",
    "set_cookie_changed",
    "keyword_changed"
  ],
  "summary": "由 401 变为 200，新增 Set-Cookie，响应中出现 welcome"
}
```

## 19. Planner 决策规则

planner 不应自由发散，建议硬编码若干稳定规则。

### 19.1 业务类型识别

- 含 `username/password` 优先弱密码与认证绕过
- 含 `id/uid/orderId/docId` 优先 ID 枚举与越权辅助
- 含 `url/redirect/callback/return` 优先 SSRF 与跳转测试
- 含搜索、过滤、排序字段优先注入测试
- 上传接口优先文件名、文件内容、路径处理测试

### 19.2 场景优先级

- 用户明确指定的场景最高优先级
- 登录接口优先弱密码
- 详情/查询接口优先 ID 枚举与 SQLi
- 回显接口优先 XSS
- 文件路径接口优先 Traversal
- URL 输入接口优先 SSRF

### 19.3 升级条件

满足以下任一条件，可从基础探测升级到深入测试：

- 出现错误栈关键字
- 响应时间明显异常
- 状态码变化显著
- 数据条目数量变化
- 回显未转义
- 登录态变化

## 20. Budget 与停止条件

为了避免 AI 无界试探，建议在 loop 内显式管理预算。

### 20.1 全局预算

- 单轮 loop 最大请求数
- 单场景最大请求数
- 单批次最大请求数
- 单 target 最大复测次数

### 20.2 停止条件

- 已确认 finding
- 场景预算耗尽
- 连续多批次无新信号
- 服务开始限流或封禁
- 用户目标已满足

## 21. Prompt 骨架建议

### 21.1 `persistent_instruction.txt`

应包含以下段落：

- 角色定义：当前是手工 HTTP 发包安全测试 loop
- 工作流要求：先分析请求面，再规划，再执行
- target_ref 规范
- action 使用规则
- fuzztag 优先策略
- 证据化输出要求
- 预算意识和停止条件

### 21.2 `reactive_data.txt`

建议渲染顺序：

1. 原始用户任务
2. 请求摘要
3. 参数清单
4. 高价值目标
5. 当前测试计划
6. 基线响应指纹
7. 最近一批结果
8. 异常候选
9. 已确认 finding
10. 剩余预算

### 21.3 `reflection_output_example.txt`

必须覆盖的示例：

- `load_http_request`
- `inspect_request_surface`
- `run_generic_vuln_test`
- `run_weak_password_test`
- `run_identifier_enumeration`
- `run_sensitive_info_exposure_test`
- `run_encoding_bypass_test`
- `commit_finding`

## 22. Profile 体系建议

建议将 profile 固定为“名称 + 适用场景 + 样本模板 + 升级关系”。

### 22.1 profile 示例

```json
{
  "name": "id_enum_zero_padded",
  "scenario": "identifier_enumeration",
  "samples": [
    "{{int(1-20|4)}}"
  ],
  "upgrade_to": ["id_enum_numeric_extended"],
  "notes": "用于四位零填充编号枚举"
}
```

### 22.2 profile 组织原则

- profile 名称稳定
- 每个 profile 只做一件事
- profile 内样本以模板表达，不直接散落到 prompt
- profile 之间可以有升级关系

## 23. 建议的最终交付形态

如果后续按本设计实现，新 loop 的核心交付应包括：

- 一套新的 loop 主体
- 一套新的 action schema
- 一套新的 prompt 模板
- 一套 target_ref 解析机制
- 一套 profile 配置
- 一套响应指纹与 finding 收敛机制

这会比在旧实现上打补丁更干净，也更适合长期演进。
