`fp` 库提供指纹（fingerprint）规则匹配能力，对响应数据应用内置或自定义规则，识别其对应的产品/组件。它是 `servicescan` 指纹识别背后的规则引擎入口。

典型使用场景：

- 规则匹配：`fp.MatchRsp(rsp)` 用全部规则匹配响应并返回命中的指纹，`fp.MatchRspByRule(rsp, rule)` 用指定规则判断是否命中。
- 规则浏览：`fp.GetAllFingerprint` 流式列出全部内置指纹规则。

与相邻库的关系：`fp` 是指纹规则层，`servicescan` 在其之上做端到端的服务识别；识别出的产品/版本可交给 `cve`/`sca` 做漏洞关联。
