# minirehs 性能计划执行记录

所有后续工作继续在 [PR #5321](https://github.com/yaklang/yaklang/pull/5321) 上推进，目标分支为 main。
各任务使用独立提交和实验基线，不把组合收益当作单项严格 A/B。

## 当前范围

用户在 2026-10-18 明确要求先完成 Windows 内容，暂不寻找其他机器或审查入口。
当前目标为 Windows/amd64、CGO_ENABLED=1、现有 clang 和 VillanCh/go-pcre2-lite v0.1.6。
Linux/amd64、macOS/arm64 原生性能和独立审查保留待补；Windows 阶段完成不代表三平台 M5 验收。
不新增第三方依赖，不新增 workflow、job 或操作系统判断。

## 身份和材料核对（T00）

- 研究基线：8911cf7d8bd75d327919f3f4e41f217dbd5dd173，独立工作副本保留。
- 合并后的 main：4373b3010a96bab70647471b28c655e6322fc778。
- 本轮起点：27100ce303e0c10d22d00ccab401de6167435fe4，已 rebase；与原 7c940fa98 的代码树相同。
- 五项既有实现：共享 PCRE subject、连续 LimEx 派生表、完整 bracket 放宽、局部 gate、anchored 首字节 mask。
  它们已在本 PR 中，后续卡片复验已有实现，不重复记为新优化。
- 实际有序规则：GetRequest 4941 条；MITM 原始 89 条，失败项单独导出，不静默删除。
- 本地语料：1332 条、5,247,590 字节有效记录内容；容器文件另含每条长度字段。
- 生产存在性/定位/早停权重、生产 SLO 尚未提供，不构造所谓平均生产收益。
- 既有测量包含正确性与性能的组合修改，不能直接作为后续 correctness-only A1。
- T00 起点为 portable image v4、native blob v3；身份任务没有改变格式或算法。

本轮可选诊断入口按现有普通测试组织，不改变 CI：

```sh
CGO_ENABLED=1 MINIREHS_EVIDENCE_DIR=/local/evidence/T00 \
  go test ./common/minirehs -run '^TestProgramCorpusManifest$' -count=1 -v
```

该入口导出 Go 实际读取的原始有序 Pattern、完整编译报告，以及每条 MITM 规则的编译错误。
默认不写文件；本地证据包含原始语料身份，不自动上传到任何服务。

## 推进顺序

T01 建立路由和阶段归因；T02 补完整元组、重复、错误与早停差分；随后以 T03–T05 建立正确性基线。
T06–T16 的每项优化先验证反例，再进行单变量 A/B；没有收益的假设可以否定。
T17 汇总 Windows 系统验证，其他平台和独立审查仍明确列为待补证据。

## Windows 阶段检查点

- T02：192 组完整结果对；comparator 可识别漏 ID、错偏移、多重复、哨兵、吞错、完成和顺序差异。
  独立 oracle 直接使用完整 subject 的 RE2 / 现有 Yak wrapper，另验证有界 PCRE 资源错误。
- T03：修复测试用途 native record batch 的 256 成员截断。255/256/257/300、多记录归属、非法偏移、
  scratch 重用、实际 Go blob 的容量 0/1 和 native 分配失败已验证；普通包、amalgamation、专项 race 通过。
  没有将该接口接入生产 Scan，没有改变 image/blob 布局。
- T04：Engine 恢复每候选一次完整 subject 的全局非重叠选择；localized gate 从全部事件取得最小终点，
  不要求 Teddy 事件排序。另修复 RuneError 字面量对非法 UTF 的漏报。
  portable image 语义版本升为 v5，旧 v1–v4 原样加载/导出，非法 UTF 时保守完整确认；cache 自动重建一次。
  nullable RE2 fallback 原有的精确 span 例外保持不变。

这些是阶段证据；T04 完整回归、T05 错误贯通及后续性能实验仍按实际任务记录推进，尚未声称 M5 完成。

## 执行错误接口（T05 进行中）

`Database.Scan` / `ScanBatch` 的执行失败可用 `errors.As` 提取 `*ScanError`，包含规则、记录、
阶段、类别及 incomplete。原绑定未暴露的数字错误码保持 nil，原错误仍可用 `errors.Is/As` 检查。
失败前的回调不撤销；Batch 的错误按原 record 顺序重放，之前的 handler=false 不观察后错。

Group 的可靠入口为 `MatchErr`、`MatchStringErr`、`MatchBytesErr`、`FindErr`、`MatchedIndexesErr`、
`MatchedPatternsErr`、`CountErr`、`ScanErr`。集合/计数入口可在 error 同时返回已观察前缀，不能当作完整结果。
旧无 error 签名保留成功行为，执行失败时 panic(ScanError)，不再伪装 NoMatch；`MatchAny` 返回错误。

```go
indexes, err := group.MatchedIndexesErr(data)
if err != nil {
    // indexes 只包含失败前已观察到的规则；按调用方的错误路径处理。
    return err
}
_ = indexes
```

Close 前由调用方收拢扫描，不在 handler 内关闭同一 DB；关闭后 Scan、ScanBatch、NewScratch 返回 ErrClosed。
PCRE2、lazy verifier、native 分配和 dispatcher 错误均已贯通。C 的 -1/-2/-3 分别保留为
invalid dispatch、overflow、out of memory；已启动 worker 在所有返回路径收拢，再按回调顺序报告。
多条 native 结果槽使用有符号状态编码，负值先作为错误处理，不能用非零直接当作命中。
trafficguard 改用 MatchedIndexesErr；候选过滤失败时对全部原规则执行其权威提取器，避免把部分候选当完整结果。

当前生产配置保留 VillanCh/go-pcre2-lite v0.1.6 的默认 match/depth 资源策略，没有新设规则或输入限制。
已有表达式内的 PCRE2 LIMIT 指令随表达式保存在镜像中；Compile/Load 都验证同样的执行错误。
专项测试使用现有公开 SetMatchLimits，在对象被扫描线程使用之前配置；该操作会重编译并替换后端，
不允许对正在扫描的实例动态调用。MatchTimeout 或在外层 select 返回不能取消正在执行的 C 匹配；
本实现不声称支持这种取消，也不访问 PCRE2 私有句柄或新增范围接口。

独立 C 故障探针使用真实 Go blob，覆盖 16 条 native 路径的 38 次 malloc/calloc 失败及临时缓冲释放；
普通 Go 回归另验证状态桥接、Compile/Load、GOMAXPROCS=1/2、早停、Batch 的 record 归属与错误后复用。
这两类证据分开记录，没有把 Go 状态注入冒充 C 内存分配故障。
池高水位和 Group 常驻 worker 所有权继续在 T06/T15 检查；本阶段尚未声称完整 M5 验收。

上一轮性能结论及边界见 [RUNTIME_PERFORMANCE.md](RUNTIME_PERFORMANCE.md)。
