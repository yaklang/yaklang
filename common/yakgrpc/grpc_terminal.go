package yakgrpc

import (
	"context"
	"os/exec"
	"runtime"
	"strings"

	"github.com/google/shlex"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func getShellCommand() (string, error) {
	switch os := runtime.GOOS; os {
	case "windows":
		return "cmd /k", nil
	case "linux", "darwin":
		var (
			finErr error
			shell  string
		)
		for _, shellName := range []string{"bash", "sh"} {
			cmd := exec.Command("which", shellName)
			shellBytes, err := cmd.CombinedOutput()
			if err == nil {
				shell = strings.TrimSpace(string(shellBytes))
				break
			} else {
				finErr = err
			}
		}

		if shell == "" && finErr != nil {
			return "", utils.Errorf("failed to find shell: %s", finErr)
		}
		return shell + " -i", nil
	default:
		return "", utils.Errorf("unsupported os: %s", os)
	}
}

func (s *Server) YaklangTerminal(inputStream ypb.Yak_YaklangTerminalServer) error {
	ctx, cancel := context.WithCancel(inputStream.Context())
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			cancel()
			return
		}
	}()

	firstInput, err := inputStream.Recv()
	if err != nil {
		return err
	}

	// exec
	shell, err := getShellCommand()
	if err != nil {
		return err
	}

	cmds, _ := shlex.Split(shell)
	cmd := exec.CommandContext(ctx, cmds[0], cmds[1:]...)
	if firstInput.GetPath() != "" {
		cmd.Path = firstInput.GetPath()
	}

	streamerRWC := &OpenPortServerStreamerHelperRWC{
		stream: inputStream,
	}
	cmd.Stdin = streamerRWC
	cmd.Stdout = streamerRWC
	cmd.Stderr = streamerRWC

	inputStream.Send(&ypb.Output{
		Control: true,
		Waiting: true,
	})

	defer func() {
		inputStream.Send(&ypb.Output{
			Control: true,
			Closed:  true,
		})
	}()
	return cmd.Run()
}
