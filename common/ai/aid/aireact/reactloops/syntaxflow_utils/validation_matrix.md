# SyntaxFlow / SSA Risk validation matrix (manual)

| Area | Check |
| --- | --- |
| Shared packages | `go build ./common/ai/aid/aireact/reactloops/...` |
| scan four paths | Attach task, new scan from path, explicit config JSON, interpret-only |
| risk review | `reload_ssa_risk` loads row; `mark_ssa_risk_disposal` writes DB |
| scan handoff actions | **TODO**: `loop_syntaxflow_scan/actions_transition.go` 已实现 open_review* / open_rule_writer* / open_code_audit* / read_ssa_project_file，但 `syntaxflow_scan` init 未注册（见 init.go TODO） |
| rule writer trial | `run_syntaxflow_rule_on_project` reachable from write_syntaxflow_rule |
