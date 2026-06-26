`nasl` 库提供对 NASL（Nessus Attack Scripting Language）脚本的加载与执行能力，可运行大量开源漏洞检测插件对目标做漏洞扫描，是兼容既有漏洞库的扫描引擎。

典型使用场景：

- 扫描：`nasl.Scan(hosts, ports, opts...)` 对目标范围扫描，`nasl.ScanTarget` 扫描单目标，返回知识库（KBs）结果 channel。
- 插件与数据库：`nasl.QueryAllScripts` 查询可用脚本，`nasl.UpdateDatabase` / `nasl.RemoveDatabase` 维护脚本库，`nasl.plugin` / `nasl.family` / `nasl.conditions` 选择脚本，`nasl.riskHandle` 处理发现的风险。

与相邻库的关系：`nasl` 与 `nuclei`（YAML 模板）同为模板化漏洞扫描引擎，发现的风险可经 `risk` 记录、`report` 汇总。
