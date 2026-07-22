# CI 基础设施（SSA 增量扫描）— 本地监控模式

在 **本地机器** 上通过 `ci-promote-monitor.py` 监控 yaklang main 分支，自动执行 SSA 增量 promote，不依赖 GitHub Actions。

## 语义

| 事件 | 动作 | 脚本 |
|------|------|------|
| 新 PR 开启 | 对最新 main 做 diff 增量扫描 | `generate-diff-scan-config.sh` + `diff-code-scan.json` |
| PR 合并到 main | 增量编译 overlay → 合并到基线 → 更新本地 main | `promote-base-on-merge.sh` |
| 每周五 | 全量编译 `ci-yaklang-base`（手动/计划任务） | `ci-yaklang-base-compile.json` |
| 日常维护 | overlay 链过深时压平；清理残留 program | `flatten-overlay.yak` / `cleanup-programs.sh` |

> Monitor 自动处理「PR 合并」；「新 PR 增量扫描」和「周五全量编译」需手动或通过外部调度触发。

---

## 架构

```
GitHub main ──poll──> ci-promote-monitor.py (每 5 分钟)
                         │
                         ├─ main HEAD == manifest sha? → 无操作
                         │
                         └─ main HEAD != manifest sha?
                              ├─ GitHub compare+blobs API 构建 fs.zip
                              ├─ promote-base-on-merge.sh 增量编译
                              ├─ 更新 manifest + pointer
                              └─ 记录事件到 events.json
```

| 角色 | 名称 | 含义 |
|------|------|------|
| 全量基线 program | `ci-yaklang-base` | 全量编译产物；周五全量后 pointer 指向它 |
| 有效基线 | `base-program-name` 文件 | 当前 promote / 扫描使用的 program 名 |
| Promote program | `ci-yaklang-promote-{sha8}` | PR 合并后增量编译出的新基线 |
| SSA 库文件 | `default-yakssa.db` | SQLite SSA 数据库 |
| 本地 manifest | `manifest.json` | 数据目录中，记录 `main_sha` / `base_program_name` / `overlay_depth` |
| 事件日志 | `events.json` | PR 生命周期事件（merged/ci_running/ci_done/closed） |

---

## 快速启动

```bash
# 1. 确保已有全量基线（首次必须手动）
cd ~/yaklang_workspace/yhellow-ssa-incremental
export SSA_CI_DATA_DIR=./ci-ssa-data
export SSA_DATABASE_RAW=$SSA_CI_DATA_DIR/default-yakssa.db
export CI_SSA_BASE_PROGRAM=ci-yaklang-base

# 2. 启动 monitor（前台或 tmux）
python3 -u scripts/ci-ssa/ci-promote-monitor.py --interval 300
```

可选环境变量：
- `GITHUB_TOKEN`：提高 API 限额（60 → 5000 req/hr）
- `CI_SSA_OVERLAY_DEPTH_LIMIT`：overlay 压平阈值（默认 5）

---

## ci-promote-monitor.py

监控 yaklang/yaklang main 分支，检测到推进时自动执行 promote。

| 参数 | 默认 | 说明 |
|------|------|------|
| `--once` | 否 | 单次检查后退出 |
| `--interval N` | 300 | 轮询间隔秒数 |
| `--repo` | yaklang/yaklang | GitHub 仓库 |
| `--worktree` | ~/yaklang_workspace/yhellow-ssa-incremental | worktree 路径 |
| `--data-dir` | ./ci-ssa-data | 数据目录 |

### fs.zip 生成方式

| 方式 | 条件 | 说明 |
|------|------|------|
| **方案 B（主路径）** | GitHub API 可用 | compare API 拿文件列表 + blobs API 拿内容，无需本地 git 历史 |
| **fallback** | API 限流/超时 | 用 `yak gitefs --start X --end Y`，需 clone 含完整 main 历史 |

### 事件记录

每次检测到 main 推进或 PR 关闭时，monitor 将事件追加到 `ci-ssa-data/events.json`：

```json
[
  {"type": "merged", "pr_number": 4790, "title": "fix(mcp)...", "sha": "a618720fb", "timestamp": "..."},
  {"type": "ci_running", "pr_number": 4790, "sha": "a618720fb", "timestamp": "..."},
  {"type": "ci_done", "pr_number": 4790, "sha": "a618720fb", "success": true, "timestamp": "..."},
  {"type": "closed", "pr_number": 4788, "title": "WIP experiment", "timestamp": "..."}
]
```

保留最近 200 条事件。

---

## Promote 流程

[promote-base-on-merge.sh](./promote-base-on-merge.sh) 执行步骤：

1. 导出环境变量（`SSA_CI_DATA_DIR` / `SSA_DATABASE_RAW` / `CI_SSA_BASE_PROGRAM`）
2. 获取 DB 写锁（flock 排他锁）
3. 校验基线 program 存在且 manifest/pointer/DB 一致
4. 生成 fs.zip（monitor 预建 或 `yak gitefs`）
5. 增量编译 `ci-yaklang-promote-{sha8}`（base = 当前 pointer）
6. 更新 pointer + manifest（`overlay_depth + 1`）
7. 清理该 PR 的 diff program
8. 如果 `overlay_depth` 超过阈值，触发 `flatten-overlay.yak` 压平

### Catch-up 模式

多个 PR 短时间内合并时，monitor 逐个 commit 走 promote，每个 PR 的 diff 独立成一层 overlay。promote 脚本也内置 catch-up 循环（`CI_SSA_PROMOTE_CATCH_UP=1`）。

### Overlay Flatten

`overlay_depth` 超过阈值（默认 5）时，promote 自动触发 [flatten-overlay.yak](./flatten-overlay.yak)：
- 提取 overlay 聚合文件系统
- 全量重编译为单层 program
- 重置 `overlay_depth=0`

手动运行：
```bash
yak scripts/ci-ssa/flatten-overlay.yak \
  --program ci-yaklang-promote-abcd1234 \
  --output ci-yaklang-base \
  --database sqlite://$SSA_DATABASE_RAW \
  --config scripts/ci-ssa/ci-yaklang-base-compile.json
```

---

## 文件说明

| 文件 | 类型 | 说明 |
|------|------|------|
| [ci-promote-monitor.py](./ci-promote-monitor.py) | Python | 主监控脚本：轮询 main，检测合并，触发 promote |
| [promote-base-on-merge.sh](./promote-base-on-merge.sh) | Shell | 核心：PR 合并 → 增量编译 → 更新基线（自包含 env/lock/check/manifest） |
| [cleanup-programs.sh](./cleanup-programs.sh) | Shell | 清理 program：`pr <N>` / `stale` / `name <prog>` |
| [generate-diff-scan-config.sh](./generate-diff-scan-config.sh) | Shell | 生成 PR 增量扫描 config |
| [flatten-overlay.yak](./flatten-overlay.yak) | Yak | overlay 链压平为单层 program |
| [remove-program.yak](./remove-program.yak) | Yak | 删除指定 program（支持 `--database`） |
| [ci-yaklang-base-compile.json](./ci-yaklang-base-compile.json) | 配置 | 全量编译模板 |
| [ci-yaklang-promote-compile.json](./ci-yaklang-promote-compile.json) | 配置 | promote 增量编译模板 |
| [diff-code-scan.json](./diff-code-scan.json) | 配置 | PR 增量扫描模板 |

### manifest 字段

```json
{
  "version": "1",
  "base_program_name": "ci-yaklang-base",
  "main_sha": "<main git SHA>",
  "overlay_depth": 0,
  "yak_version": "<yak version>",
  "database": { "url": "local://...", "size_bytes": 0, "compression": "none" },
  "updated_at": "2026-01-01T00:00:00Z"
}
```

### events.json 字段

| type | 字段 | 说明 |
|------|------|------|
| `merged` | pr_number, title, sha, html_url | PR 合并到 main |
| `ci_running` | pr_number, sha | promote 开始执行 |
| `ci_done` | pr_number, sha, success | promote 完成（success: true/false） |
| `closed` | pr_number, title, html_url | PR 关闭（非合并） |

---

## Go 桥接函数

`ssaapi.Exports` 中注册的脚本可用函数（`common/yak/ssaapi/ssa_flatten.go`）：

| 导出名 | 说明 |
|--------|------|
| `ssa.SetDatabase` | 设置活跃 SSA 库 |
| `ssa.GetOverlayFiles` | 提取 overlay 聚合 FS |
| `ssa.DeleteProgram` | 删除 program |
| `ssa.ListPrograms` | 列出所有 program |

---

## 故障排查

| 现象 | 原因 | 处理 |
|------|------|------|
| `Base program not found` | 未跑全量或 pointer 指向已删 program | 手动全量编译 |
| `Base pointer drift` | manifest/pointer/DB 不一致 | 全量重编译纠偏 |
| Promote `ancestor` 失败 | main 历史改写 | 全量重编译 |
| `compare API 403` | GitHub API 限流 | monitor 自动 fallback 到 yak gitefs |
| `DNS failure` | 网络问题 | monitor 自动 fallback 到 git fetch |
| `program existed` | 重试时同名 program 冲突 | promote 脚本自动清理旧 program |