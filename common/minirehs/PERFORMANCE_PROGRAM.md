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
- portable image v4、native blob v3；T00 没有改变格式或算法。

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

上一轮性能结论及边界见 [RUNTIME_PERFORMANCE.md](RUNTIME_PERFORMANCE.md)。
