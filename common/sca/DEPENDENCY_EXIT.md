# SCA 依赖退出报告

基线 `d33a21b6a30945df93a663a18f3cce087b0f3718` 的 SCA 生产闭包：**704 个包、91 个第三方模块**。当前：**134 个包、0 个第三方模块**；含测试闭包当前 **205 个包、0 个第三方模块**。详见 audit 的原始门禁结果。

审计解析全部 `go list -deps -json` 对象，遇到 Error/DepsErrors/未知分类即失败。SCAOwned 以外只允许 `common/utils/filesys/filesys_interface`，其传递闭包也递归检查。算法核心、公共入口、自有 SBOM 及兼容适配全部在同一严格闭包内，没有依赖 build tag 隐藏旧实现。

`go mod tidy` 移除了 CycloneDX SDK、go-containerregistry、go-rpmdb、jfather、x/xerrors、estargz、tar-split、go-isatty 的 require。没有升级这些来源来迁测试。pty/runewidth 的 direct 标记及 aec 的 indirect 声明是 tidy 对全仓既有使用关系的归整。

## 全仓仍保留的使用者

这些模块已退出 SCA，但仍被其他功能使用。以下为当前 `go mod why -m all` 返回的实际最短引用链；完整 91 项清单在 `audit/dependency-exit.json`。

- `github.com/BurntSushi/toml`：`github.com/yaklang/yaklang/common/urfavecli/altsrc` → `github.com/BurntSushi/toml`.
- `golang.org/x/mod`：`github.com/yaklang/yaklang/common/utils/lowhttp` → `github.com/quic-go/quic-go` → `go.uber.org/mock/mockgen` → `golang.org/x/mod/modfile`.
- `gopkg.in/yaml.v3`：`github.com/yaklang/yaklang/common/ai/aid/aicommon` → `gopkg.in/yaml.v3`.
- `github.com/docker/docker`：`github.com/yaklang/yaklang/common/thirdpartyservices` → `github.com/docker/docker/api/types`.
- `github.com/docker/go-connections`：`github.com/yaklang/yaklang/common/thirdpartyservices` → `github.com/docker/go-connections/nat`.
- `github.com/mattn/go-sqlite3`：`github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader` → `github.com/mattn/go-sqlite3`.
- `github.com/samber/lo`：`github.com/yaklang/yaklang/common/ai` → `github.com/samber/lo`.
- `golang.org/x/text`：`github.com/yaklang/yaklang/common/yak/yaklib/codec` → `golang.org/x/text/encoding`.
- `golang.org/x/net`：`github.com/yaklang/yaklang/common/gmsm/gmtls/gmcredentials` → `golang.org/x/net/context`.

## 源码与模块分开计量

零运行模块不等于没有第三方来源代码。x/mod 的只读词法、TOML 必要 lexer/scalar，以及部分格式记录算法和测试来自锁定来源。源码映射、完整提交、文件摘要、许可证分别在 `source-extraction-map.json`、`upstream-sources.lock.json`、`SCA_THIRD_PARTY_NOTICES.md`。参考 checkout 留在产品模块外。

旧调度器、Docker/Git 获取链路、旧 Go mod/sum 包、Cargo 二次行扫描、全生态版本猜配和 SDK 类型已退出。只保留必要的旧函数名作为精确身份兼容转换，没有运行时 fallback。

源码门禁使用 Go AST 检查 import、selector 和原生代码指令；包图门禁检验传递闭包。隔离运行门禁禁止网络、写文件和子进程（仅允许初始测试二进制执行）。macOS sandbox 的成功退出不能单独证明零次被拦截尝试；源码与隔离两项证据分别报告。
