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

	// exec
	shell, err := getShellCommand()
	if err != nil {
		return err
	}

	cmds, _ := shlex.Split(shell)
	cmd := exec.CommandContext(ctx, cmds[0], cmds[1:]...)

	streamerRWC := &OpenPortServerStreamerHelperRWC{
		stream: inputStream,
	}
	cmd.Stdin = streamerRWC
	cmd.Stdout = streamerRWC
	cmd.Stderr = streamerRWC

	return cmd.Run()
	// stdin, _ := cmd.StdinPipe()
	// stdout, _ := cmd.StdoutPipe()
	// stderr, _ := cmd.StderrPipe()
	// defer func() {
	// 	stdin.Close()
	// 	stdout.Close()
	// }()
	// cmd.Start()
	// go func() {
	// 	defer cancel()
	// 	_, err := io.Copy(streamerRWC, stdout)
	// 	if err != nil {
	// 		log.Errorf("stream copy stdout from local process to grpcChannel failed: %s", err)
	// 	}
	// 	// log.Infof("finished for conn %v <-- %v ", addr, conn.RemoteAddr())
	// 	streamerRWC.stream.Send(&ypb.Output{
	// 		Control: true,
	// 		Closed:  true,
	// 	})
	// }()
	// go func() {
	// 	defer cancel()
	// 	_, err := io.Copy(streamerRWC, stderr)
	// 	if err != nil {
	// 		log.Errorf("stream copy stderr from local process to grpcChannel failed: %s", err)
	// 	}
	// }()

	// go func() {
	// 	defer cancel()
	// 	_, err := io.Copy(stdin, streamerRWC)
	// 	if err != nil {
	// 		log.Errorf("stream copy stdin from grpcChannel to local process failed: %s", err)
	// 	}
	// 	streamerRWC.stream.Send(&ypb.Output{
	// 		Control: true,
	// 		Closed:  true,
	// 	})
	// }()
	// cmd.Wait()
	// return nil
}
