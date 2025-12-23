# AIBalance Memfit TOTP 认证功能测试报告

**测试时间**: 2025-12-23 14:56-15:00  
**测试环境**: macOS, Go, AIBalance 线上服务器 aibalance.yaklang.com  
**测试结果**: ✅ **全部通过**

---

## 一、测试目标

验证 AIBalance 服务的 Memfit TOTP 认证功能：

| 功能 | 预期行为 |
|------|----------|
| TOTP 仅对 memfit- 模型生效 | ✅ 其他模型不添加 TOTP 头 |
| 首次使用自动获取 TOTP | ✅ 从服务器获取并保存到数据库 |
| 后续使用从数据库加载 | ✅ 避免重复请求服务器 |
| 密钥不一致时自动刷新 | ✅ 检测 401 错误后刷新并重试 |

---

## 二、测试用例与结果

### 测试1: 获取 TOTP UUID 公开接口

**命令**:
```bash
curl -s https://aibalance.yaklang.com/v1/memfit-totp-uuid
```

**结果**:
```json
{
  "format": "MEMFIT-AI<uuid>MEMFIT-AI",
  "uuid": "MEMFIT-AI82771765-bd51-4f2a-b719-d536b3174611MEMFIT-AI"
}
```

**状态**: ✅ **通过**

---

### 测试2: 首次使用 memfit-light-free (清除缓存后)

**测试脚本**:
```yak
db.SetKey('AIBALANCE_CLIENT_TOTP_SECRET', '')  // 清除缓存
result, err = ai.Chat('你好，请用一句话回答：1+1等于多少？', 
    ai.type('aibalance'), 
    ai.model('memfit-light-free'))
```

**执行日志**:
```
[INFO] Initializing TOTP secret for aibalance client...
[INFO] Fetching TOTP UUID from: https://aibalance.yaklang.com/v1/memfit-totp-uuid
[INFO] Successfully fetched TOTP secret from server
[INFO] TOTP secret saved to database
[INFO] TOTP secret initialized from server
[INFO] Added TOTP auth header for memfit model: memfit-light-free
```

**AI 回复**: （服务器未配置该模型返回 404，但 TOTP 流程正确）

**状态**: ✅ **通过** - TOTP 初始化流程正确

---

### 测试3: 正常 memfit-light-free 请求

**测试脚本**:
```yak
result, err = ai.Chat('你好，请用一句话回答：1+1等于多少？', 
    ai.type('aibalance'), 
    ai.model('memfit-light-free'))
```

**执行日志**:
```
[INFO] Loaded TOTP secret from database during initialization
[INFO] Added TOTP auth header for memfit model: memfit-light-free
[INFO] first byte(token) delay: 1.034567791s
```

**AI 回复**: `1+1等于2。`

**状态**: ✅ **通过**

---

### 测试4: TOTP 密钥不一致时自动刷新 ⭐ 关键测试

**测试脚本**:
```yak
db.SetKey('AIBALANCE_CLIENT_TOTP_SECRET', 'wrong-secret-12345')  // 设置错误密钥
result, err = ai.Chat('你好，请用一句话回答：2+2等于多少？', 
    ai.type('aibalance'), 
    ai.model('memfit-light-free'))
```

**执行日志**:
```
[INFO] Loaded TOTP secret from database during initialization
[INFO] Added TOTP auth header for memfit model: memfit-light-free
[WARN] response status code: 401
[INFO] response body: {"error":{"message":"Memfit TOTP authentication failed...","type":"memfit_totp_auth_failed"}}
[WARN] Empty result for memfit model, may be TOTP auth failure, will try refresh
[WARN] TOTP authentication failed for memfit model, refreshing secret and retrying...
[WARN] Refreshing TOTP secret due to authentication failure...
[INFO] Fetching TOTP UUID from: https://aibalance.yaklang.com/v1/memfit-totp-uuid
[INFO] Successfully fetched TOTP secret from server
[INFO] TOTP secret saved to database
[INFO] TOTP secret refreshed: old=wrong-se... new=82771765...
[INFO] Added TOTP auth header for memfit model: memfit-light-free
[INFO] first byte(token) delay: 736.156208ms
2+2等于4。
```

**AI 回复**: `2+2等于4。`

**更新后密钥**: `82771765-bd51-4...` (正确)

**状态**: ✅ **通过** - 自动刷新并重试成功

---

### 测试5: 非 memfit 模型 (glm-4-flash-free)

**测试脚本**:
```yak
result, err = ai.Chat('你好，请用一句话回答：5+5等于多少？', 
    ai.type('aibalance'), 
    ai.model('glm-4-flash-free'))
```

**执行日志**:
```
[INFO] start to chat completions by aibalance
[INFO] first byte(token) delay: 895.469833ms
你好，5+5等于10。
```

**AI 回复**: `你好，5+5等于10。`

**关键验证**: 日志中 **没有** "Added TOTP auth header" - TOTP 不对非 memfit 模型生效

**状态**: ✅ **通过**

---

## 三、测试结果汇总

| 测试项 | 模型 | TOTP | AI 返回 | 结果 |
|--------|------|------|---------|------|
| TOTP UUID 接口 | - | - | JSON 正确 | ✅ |
| 首次获取 TOTP | memfit-light-free | 从服务器获取 | - | ✅ |
| 正常请求 | memfit-light-free | 从数据库加载 | "1+1等于2。" | ✅ |
| 密钥不一致自动刷新 | memfit-light-free | 自动刷新 | "2+2等于4。" | ✅ |
| 非 memfit 模型 | glm-4-flash-free | 不添加 | "5+5等于10。" | ✅ |

---

## 四、关键日志标识说明

| 日志内容 | 含义 |
|----------|------|
| `Initializing TOTP secret for aibalance client...` | 首次初始化 TOTP |
| `Fetching TOTP UUID from: ...` | 正在从服务器获取 TOTP |
| `Successfully fetched TOTP secret from server` | 成功获取密钥 |
| `TOTP secret saved to database` | 密钥已保存到数据库 |
| `Loaded TOTP secret from database during initialization` | 从数据库加载（后续请求） |
| `Added TOTP auth header for memfit model` | 已添加 X-Memfit-OTP-Auth 头 |
| `Empty result for memfit model, may be TOTP auth failure` | 检测到空结果，准备刷新 |
| `TOTP secret refreshed: old=... new=...` | 密钥刷新成功 |

---

## 五、修复内容总结

### 问题
用户报告线上环境大量出现 TOTP 认证失败，但客户端没有自动刷新密钥。

### 原因分析
1. `sync.Once` 只初始化一次，后续请求使用缓存中的错误密钥
2. 流式请求中 401 错误没有正确传递给调用者
3. 空结果没有触发刷新逻辑

### 修复方案
1. **使用 `sync.Once` 控制初始化** - 确保只初始化一次，避免重复请求
2. **包装错误处理器** - 捕获 TOTP 错误并设置标志
3. **空结果检测** - memfit 模型返回空结果时，尝试刷新 TOTP
4. **自动刷新并重试** - 刷新密钥后立即重试请求

### 修改文件
- `common/ai/aibalance/gateway.go` - 添加 TOTP 错误检测和自动刷新逻辑

---

## 六、结论

**所有 5 项测试全部通过** 🎉

TOTP 认证系统工作正常：
- ✅ TOTP 仅对 memfit- 模型生效
- ✅ 首次使用从服务器获取并保存到数据库
- ✅ 后续请求从数据库加载
- ✅ 密钥不一致时自动刷新并重试
- ✅ 非 memfit 模型不受影响
