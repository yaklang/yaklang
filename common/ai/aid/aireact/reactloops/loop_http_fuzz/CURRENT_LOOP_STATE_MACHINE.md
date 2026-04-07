# Current HTTP Fuzz Loop State Machine

本文档描述当前 `http_fuzz` loop 的实际实现流程，对应代码主要在：

- `init.go`
- `actions.go`
- `state.go`

## Full State Machine

```mermaid
stateDiagram-v2
    [*] --> Idle

    Idle --> LoadRequest: load_http_request
    LoadRequest --> RequestReady: 请求解析成功\n生成 request_profile / parameter_inventory\n建立 baseline_fingerprint
    LoadRequest --> Failed: 请求非法 / 解析失败

    RequestReady --> SurfaceInspect: inspect_request_surface
    SurfaceInspect --> Planning: 基于 high_value_targets / test_plan 选择下一步

    Planning --> PreciseMutation: mutate_target
    Planning --> GenericScenario: run_generic_vuln_test
    Planning --> WeakPassword: run_weak_password_test
    Planning --> IdentifierEnum: run_identifier_enumeration
    Planning --> SensitiveProbe: run_sensitive_info_exposure_test
    Planning --> EncodingBypass: run_encoding_bypass_test
    Planning --> End: 用户目标已满足 / 不再继续

    PreciseMutation --> BatchExecute: execute_test_batch\nvariant_source=last_mutation
    PreciseMutation --> Failed: target_ref 非法 / payload 为空

    GenericScenario --> BatchAnalyze
    WeakPassword --> BatchAnalyze
    IdentifierEnum --> BatchAnalyze
    SensitiveProbe --> BatchAnalyze
    EncodingBypass --> BatchAnalyze
    BatchExecute --> BatchAnalyze

    BatchAnalyze --> NoSignal: 无明显异常
    BatchAnalyze --> HasAnomaly: 产生 anomaly_candidates
    BatchAnalyze --> Failed: 执行异常 / 请求发送失败

    NoSignal --> Planning: 换 target / 换 scenario / 继续覆盖
    HasAnomaly --> RetestOrDeepen: 继续复测或加深测试
    RetestOrDeepen --> Planning: 再次执行其他批次
    RetestOrDeepen --> CommitFinding: commit_finding

    CommitFinding --> FindingCommitted: 写入 confirmed_findings
    FindingCommitted --> Planning: 继续扩展验证
    FindingCommitted --> End: 收敛结束

    Failed --> Planning: 调整参数后重试
    Failed --> End: 放弃当前流程

    End --> [*]
```

## Main Path

```mermaid
stateDiagram-v2
    [*] --> load_http_request
    load_http_request --> inspect_request_surface
    inspect_request_surface --> choose_strategy

    choose_strategy --> mutate_target
    mutate_target --> execute_test_batch
    execute_test_batch --> analyze_result

    choose_strategy --> run_generic_vuln_test
    choose_strategy --> run_weak_password_test
    choose_strategy --> run_identifier_enumeration
    choose_strategy --> run_sensitive_info_exposure_test
    choose_strategy --> run_encoding_bypass_test

    run_generic_vuln_test --> analyze_result
    run_weak_password_test --> analyze_result
    run_identifier_enumeration --> analyze_result
    run_sensitive_info_exposure_test --> analyze_result
    run_encoding_bypass_test --> analyze_result

    analyze_result --> choose_strategy: 无异常 / 继续覆盖
    analyze_result --> commit_finding: 异常足够明确
    commit_finding --> [*]
```

## Notes

- 当前实现更偏“收敛式循环”，不是严格硬编码的终态机。
- `End` 通常来自 AI 判断“目标已满足”或“继续收益较低”。
- `commit_finding` 是当前实现里最明确的收敛动作。
- 即使出现 `Failed`，很多情况下也会回到 `Planning`，而不是直接终止整个 loop。
