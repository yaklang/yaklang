# CI 基础设施（SSA 增量扫描）

在 **GitHub Self-hosted Runner** 上维护持久 SSA 数据库，实现：

- **每周五**：对 `main` 分支做 **yaklang 全量** golang 编译，写入基线 program `ci-yaklang-base`
- **每个 PR**：基于基线做 **增量编译 + SyntaxFlow 扫描**（`diff-code-check`）

`check-wip` 等轻量步骤仍在 GitHub 托管 runner（`ubuntu-22.04`）上执行；**编译与扫描**在自建机上执行。

---

## 架构概览

```mermaid
flowchart TB
  subgraph hosted [GitHub Hosted]
    WIP[check-wip pre_check]
  end

  subgraph selfhosted [Self-hosted runner]
    DB[("/data/ci-ssa/default-yakssa.db")]
    Weekly[CI SSA Base Weekly]
    PR[Diff-Code-Check setup]
    Weekly -->|"ssa-compile 全量"| DB
    PR -->|"gitefs + code-scan 增量"| DB
  end

  WIP --> PR
  Main[origin/main] --> Weekly
```

| 角色 | 名称 | 含义 |
|------|------|------|
| 基线 program | `ci-yaklang-base` | 每周从 `main` 全量编译得到，存于本地 SSA DB |
| PR diff program | `ci-yaklang-diff-pr-{N}-{sha8}` | 每次 PR 扫描生成的增量 layer，名由脚本生成 |
| SSA 库文件 | `$SSA_CI_DATA_DIR/default-yakssa.db` | 默认 `/data/ci-ssa/default-yakssa.db` |

---

## Workflows

| Workflow | 文件 | 触发 | Runner | 说明 |
|----------|------|------|--------|------|
| CI Infra Smoke | [ci-infra-smoke.yml](../../.github/workflows/ci-infra-smoke.yml) | PR（改 ci 相关路径）、手动 | `ubuntu-22.04` + 可选 self-hosted | 连通性探测；`self-hosted-smoke` **仅**手动 |
| CI SSA Base Weekly | [ci-ssa-base-weekly.yml](../../.github/workflows/ci-ssa-base-weekly.yml) | **周五 20:00 UTC**、`workflow_dispatch` | `[self-hosted, linux, ssa-ci]` | 全量编译并更新 manifest artifact |
| Diff-Code-Check | [diff-code-check.yml](../../.github/workflows/diff-code-check.yml) | PR → `main`（路径过滤） | `check-wip`: hosted；`setup`: self-hosted | 安全扫描与 PR 评论 |

### Diff-Code-Check 路径过滤（会触发）

- `common/**`
- `.github/workflows/diff-code-check.yml`、`ci-ssa-base-weekly.yml`
- `scripts/ci-ssa/**`、`scripts/ssa-risk-tools/**`、`scripts/get-yak-version.sh`
- `common/ssa_bootstrapping/ci_rule/**`

### 安全策略

- `setup` job **不对 fork PR 运行**（`head.repo.full_name == github.repository`）
- 自建机会执行 PR 代码，请勿对不可信 fork 放开 self-hosted job

---

## 与 Git 的对应关系（重要）

| 阶段 | Git 语义 | CI 行为 |
|------|----------|---------|
| 周五基线 | `main` 当前 tip | `checkout main` → 全量编译 → `manifest.main_sha` 记录该 commit |
| PR 变更集 | `merge-base(main, PR_HEAD) .. PR_HEAD` | `yak gitefs --start $MERGE_BASE --end $HEAD_SHA` → `fs.zip` |
| 增量编译 | 相对 **DB 中的 base program** | `base_program_name: ci-yaklang-base`，引擎对 `fs.zip` 与 base 做文件 diff |

说明：基线是「最近一次 weekly 的 main」，PR 的 merge-base 通常接近 main，但不一定等于 `manifest.main_sha`。若 main 快进很多且长期未跑 weekly，可能出现基线偏旧；此时应 **手动再跑一次 CI SSA Base Weekly**。

---

## 一次性部署（自建机）

### 1. 机器与系统

| 场景 | CPU | 内存 | 磁盘 | 系统 |
|------|-----|------|------|------|
| 试跑 weekly | 4C | 16G | ≥100G | Ubuntu **22.04** amd64 |
| 推荐生产 | 8C | 32G | ≥200G 数据盘 | Ubuntu 22.04 amd64 |

需稳定 **出站**：`github.com`、`aliyun-oss.yaklang.com`（下载 yak 二进制）。

### 2. 安装 GitHub Actions Runner

1. 仓库 **Settings → Actions → Runners → New self-hosted runner**
2. 按官方说明在 Linux 上安装 [actions-runner](https://github.com/actions/runner)
3. 注册时添加 labels（与 workflow 一致）：
   - `self-hosted`
   - `linux`
   - `ssa-ci`
4. 建议用 **systemd** 托管 runner，保证重启后在线

### 3. SSA 数据目录

```bash
sudo mkdir -p /data/ci-ssa
sudo chown "$(whoami):$(whoami)" /data/ci-ssa
```

可选：在仓库 **Settings → Variables** 新增 `SSA_CI_DATA_DIR`（例如 `/data/ci-ssa`）。  
未设置时脚本默认 `/data/ci-ssa`，数据库文件为：

```text
/data/ci-ssa/default-yakssa.db
```

环境变量（由 [export-ssa-db-env.sh](../../scripts/ci-ssa/export-ssa-db-env.sh) 导出）：

| 变量 | 含义 |
|------|------|
| `SSA_CI_DATA_DIR` | 数据根目录 |
| `SSA_DATABASE_RAW` | SQLite SSA 库完整路径 |
| `CI_SSA_BASE_PROGRAM` | 基线 program 名，默认 `ci-yaklang-base` |

### 4. 首次生成基线（必做）

1. Actions → **CI SSA Base Weekly** → **Run workflow**
2. 等待完成（全仓 golang 编译，可能 **数小时**，视机器而定）
3. 确认步骤 **Verify base program** 通过
4. 在 Artifacts 下载 **ci-ssa-manifest**，核对 `main_sha`、`size_bytes`

未完成此步前，PR 的 **Diff-Code-Check** 会在 `ensure-base-program.sh` 失败并提示先跑 weekly。

### 5. 验证（可选）

| 步骤 | 操作 |
|------|------|
| 托管 smoke | 开 PR 改 `docs/ci-infra` 或 `scripts/ci-ssa`，看 **hosted-smoke** / **storage-probe** |
| 自建 smoke | 手动 Run **CI Infra Smoke**，看 **self-hosted-smoke** |
| PR 扫描 | 开修改 `common/**` 的 PR，看 **Diff-Code-Check** |

---

## 每周五全量 job 做了什么

[ci-ssa-base-weekly.yml](../../.github/workflows/ci-ssa-base-weekly.yml) 步骤摘要：

1. `checkout` **`main`**
2. `install-yak-ci.sh` — 从 OSS 拉取与 diff-check 相同策略的 yak 版本
3. `yak sf-import` — 导入 `common/ssa_bootstrapping/ci_rule/`
4. `yak ssa-compile --config ci-yaklang-base-compile.json --database $SSA_DATABASE_RAW --re-compile`
5. `ensure-base-program.sh` — 确认 `ci-yaklang-base` 已写入 DB
6. 生成 [manifest.json](../../scripts/ci-ssa/manifest.json) 并上传 artifact **ci-ssa-manifest**（保留 90 天）

全量配置见 [ci-yaklang-base-compile.json](../../scripts/ci-ssa/ci-yaklang-base-compile.json)：`CodeSource.local_file` 为仓库根 `.`，语言 `golang`，含与 diff-check 一致的 `exclude_files`。

---

## PR 增量扫描做了什么

[diff-code-check.yml](../../.github/workflows/diff-code-check.yml) 的 `setup` job（self-hosted）：

1. 安装 yak、设置 `SSA_DATABASE_RAW`
2. `git checkout` PR head，`gitefs` 生成 `fs.zip`
3. `ensure-base-program.sh`
4. `generate-diff-scan-config.sh` → `scan-config.json`（program 名 `ci-yaklang-diff-pr-{PR}-{sha8}`）
5. `yak code-scan --config scan-config.json --database $SSA_DATABASE_RAW`

相对旧版的变化：

- 编译侧 **不再**使用 `--memory`（必须写持久 DB 才能增量）
- 使用 [diff-code-scan.json](../../scripts/ci-ssa/diff-code-scan.json) 中的 `enable_incremental_compile` + `base_program_name`

---

## `scripts/ci-ssa/` 文件说明

| 文件 | 类型 | 说明 |
|------|------|------|
| [export-ssa-db-env.sh](../../scripts/ci-ssa/export-ssa-db-env.sh) | Shell | `source` 后设置 `SSA_CI_DATA_DIR` / `SSA_DATABASE_RAW` |
| [install-yak-ci.sh](../../scripts/ci-ssa/install-yak-ci.sh) | Shell | 下载安装 yak（`get-yak-version.sh` 版本策略） |
| [ensure-base-program.sh](../../scripts/ci-ssa/ensure-base-program.sh) | Shell | 检查 DB 文件与 `ci-yaklang-base` 是否存在 |
| [generate-diff-scan-config.sh](../../scripts/ci-ssa/generate-diff-scan-config.sh) | Shell | 由模板生成 PR 专用 `scan-config.json` |
| [ci-yaklang-base-compile.json](../../scripts/ci-ssa/ci-yaklang-base-compile.json) | 配置 | 周五全量编译 |
| [diff-code-scan.json](../../scripts/ci-ssa/diff-code-scan.json) | 配置 | PR 增量扫描模板 |
| [manifest.json](../../scripts/ci-ssa/manifest.json) | 元数据 | 仓库内占位；**真实** manifest 由 weekly job 写入 artifact |
| [manifest.example.json](../../scripts/ci-ssa/manifest.example.json) | 示例 | OSS 分发场景字段示例（`PLACEHOLDER` URL） |

### manifest 字段（weekly 产出）

```json
{
  "version": "1",
  "base_program_name": "ci-yaklang-base",
  "main_sha": "<main 的 git SHA>",
  "yak_version": "<yak version 输出>",
  "database": {
    "url": "local:///data/ci-ssa/default-yakssa.db",
    "sha256": "",
    "size_bytes": 123456789,
    "compression": "none"
  },
  "updated_at": "2026-01-01T00:00:00Z"
}
```

`database.url` 以 `local://` 开头表示库在自建机本地；[ci-infra-smoke](../../.github/workflows/ci-infra-smoke.yml) 的 storage-probe 会跳过此类 URL。

---

## 故障排查

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| Job 一直 **pending** | Runner 离线或缺少 label | 检查 `self-hosted` / `linux` / `ssa-ci` |
| `Base program 'ci-yaklang-base' not found` | 未跑 weekly 或 DB 路径不对 | 手动 **CI SSA Base Weekly**；检查 `SSA_CI_DATA_DIR` |
| `SSA database not found` | `/data/ci-ssa` 未创建 | 建目录并赋权 runner 用户 |
| 全量 **OOM** / 被杀 | 内存不足 | 升到 32G+ 或减小编译范围（改 `ci-yaklang-base-compile.json`） |
| 磁盘满 | DB + workspace 过大 | 扩盘；必要时清理旧 diff program |
| PR 扫描编译失败 | 空 diff、yak 版本、基线过旧 | 看日志；重跑 weekly；确认 `fs.zip` 非空 |
| fork PR 不跑 | 故意限制 | 预期行为；仅在 upstream 仓库 PR 跑 self-hosted |

---

## 可选演进（未默认启用）

| 方向 | 说明 |
|------|------|
| **OSS / R2 备份** | 将 `default-yakssa.db.zst` 上传对象存储，manifest 改为 HTTPS；适合灾备或多机恢复 |
| **托管 runner + cache** | 无自建机时从 OSS 拉库；GitHub cache 约 10GB /repo 上限 |
| **缩小全量范围** | 将 `local_file` 从 `.` 改为子目录，缩短 weekly 时间与 DB 体积 |
| **manifest 入仓** | 每周 bot commit `manifest.json`，便于审计 `main_sha` |

---

## 相关代码（引擎行为）

增量编译由 SSA 配置驱动，见 `common/yak/ssaapi/ssaconfig/compile.go` 中 `enable_incremental_compile`、`base_program_name`。  
`code-scan --config` 走 `preferConfigCompile` 路径（`common/yak/cmd/yakcmds/ssacli_sfscan.go`），需同时指定 `--database` 指向持久库。

---

## 维护清单（建议）

- [ ] Runner 进程健康、磁盘使用率 &lt; 80%
- [ ] 每周五确认 **CI SSA Base Weekly** 成功
- [ ] main 有大版本变更后，可手动 trigger weekly
- [ ] 升级 yak 版本策略时，同步观察 diff-check 与 weekly 日志
