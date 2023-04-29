package vulinbox

import (
	"context"
	"fmt"
	"github.com/google/shlex"
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
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		outputs, err := exec.CommandContext(ctx, list[0], list[1:]...).CombinedOutput()
		writer.Write(outputs)
		if err != nil {
			writer.Write([]byte(`` + err.Error()))
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
		outputs, err := exec.CommandContext(ctx, `bash`, "-c", raw).CombinedOutput()
		writer.Write(outputs)
		if err != nil {
			writer.Write([]byte(`` + err.Error()))
			return
		}
	})
}
