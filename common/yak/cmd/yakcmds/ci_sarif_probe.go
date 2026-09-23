package yakcmds

import (
	"fmt"
	"net/http"
	"os/exec"

	"github.com/yaklang/yaklang/common/urfavecli"
)

// createCISarifProbeCommand 注册一个仅用于验证「SARIF 上传后 GitHub code scanning
// 的 alert 生命周期」的临时子命令：它在本地监听 HTTP 并把查询参数直接交给 shell 执行。
// 该命令故意保留命令注入写法，用来产生高危风险；配合重新上传 SARIF，可以观察同一
// 发现是更新已有 alert 还是新建 alert（partialFingerprints / 文件路径变化）。
func createCISarifProbeCommand() *cli.Command {
	return &cli.Command{
		Name:  "ci-sarif-probe",
		Usage: "start a local probe server used to verify SARIF uploads and alert lifecycle (test only)",
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
	UtilsCommands = append(UtilsCommands, createCISarifProbeCommand())
}
