package vulinbox

import (
	"context"
	"fmt"
	"github.com/google/shlex"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"os/exec"
	"time"
)

func (s *VulinServer) registerPingCMDI() {
	r := s.router
	r.HandleFunc("/ping/cmd/shlex", func(writer http.ResponseWriter, request *http.Request) {
		ip := request.URL.Query().Get("ip")
		if ip == "" {
			writer.Write([]byte(`no ip set`))
			return
		}
		var raw = fmt.Sprintf("ping %v", ip)
		list, err := shlex.Split(raw)
		if err != nil {
			writer.Write([]byte(`shlex parse failed: ` + err.Error()))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		outputs, err1 := exec.CommandContext(ctx, list[0], list[1:]...).CombinedOutput()
		// 尝试将 GBK 转换为 UTF-8
		utf8Outputs, err2 := utils.GbkToUtf8(outputs)
		if err2 != nil {
			writer.Write(outputs)
		} else {
			writer.Write(utf8Outputs)
		}
		if err1 != nil {
			writer.Write([]byte("exec : " + err1.Error()))
			return
		}
	})
	r.HandleFunc("/ping/cmd/bash", func(writer http.ResponseWriter, request *http.Request) {
		ip := request.URL.Query().Get("ip")
		if ip == "" {
			writer.Write([]byte(`no ip set`))
			return
		}
		var raw = fmt.Sprintf("ping %v", ip)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		outputs, err1 := exec.CommandContext(ctx, `bash`, "-c", raw).CombinedOutput()
		// 尝试将 GBK 转换为 UTF-8
		utf8Outputs, err2 := utils.GbkToUtf8(outputs)
		if err2 != nil {
			writer.Write(outputs)
		} else {
			writer.Write(utf8Outputs)
		}
		if err1 != nil {
			writer.Write([]byte("exec : " + err1.Error()))
			return
		}
	})
}
