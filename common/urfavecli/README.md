# urfave/cli (v1) — 本仓库内嵌副本

本目录为 [github.com/urfave/cli](https://github.com/urfave/cli) **v1** 的代码副本，供 yaklang 项目内部使用。

- **上游**：`github.com/urfave/cli`，v1 分支（v1-maint）
- **上游状态**：v1 已不再积极维护，上游主推 v2（包路径为 `github.com/urfave/cli/v2`）
- **本仓库**：基于上游 v1.22.15 复制并改为使用路径 `github.com/yaklang/yaklang/common/urfavecli`，便于在 yaklang 内直接维护与修改

## 使用方式

在 yaklang 代码中引用：

```go
import (
	"github.com/yaklang/yaklang/common/urfavecli"
	// 若需 altsrc（如从配置文件加载 flag）
	"github.com/yaklang/yaklang/common/urfavecli/altsrc"
)
```

## 文档

- 上游 v1 使用说明：[cli.urfave.org v1](https://cli.urfave.org/v1/getting-started/)
- 本目录内文档：[./docs/v1/manual.md](./docs/v1/manual.md)

## 构建标签

可选构建标签：

- **`urfave_cli_no_docs`**：去掉 `ToMarkdown`、`ToMan` 等文档相关方法，可减少约 300–400 KB 二进制体积（依赖更少）。

## 许可证

与上游一致，见 [LICENSE](./LICENSE)。
