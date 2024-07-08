package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/aymanbagabas/go-pty"
	"github.com/google/shlex"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/term"
)

func getShellCommand() (string, string, error) {
	var (
		finErr, err          error
		shell                string
		shellOpts            string
		shellNames           []string
		el                   string
		needReplaceBackslash bool
	)

	switch goos := runtime.GOOS; goos {
	case "windows":
		el = "\r\n"
		shellNames = []string{"powershell", "cmd"}
		needReplaceBackslash = true
	case "linux", "darwin":
		el = "\n"
		shellNames = []string{"bash", "sh"}
		shellOpts = " -i"
	default:
		return "", "", utils.Errorf("unsupported os: %s", goos)
	}

	for _, shellName := range shellNames {
		shell, err = exec.LookPath(shellName)
		if err == nil {
			break
		} else {
			finErr = err
		}
	}

	if shell == "" && finErr != nil {
		return "", "", utils.Errorf("failed to find shell: %s", finErr)
	}
	if needReplaceBackslash {
		// for windows
		shell = strings.ReplaceAll(shell, "\\", "\\\\")
	}
	return shell + shellOpts, el, nil
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
	var (
		path          string
		width, height int
	)
	if firstInput.GetPath() != "" {
		path = firstInput.GetPath()
	} else {
		p, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = p
	}

	if firstInput.GetWidth() != 0 && firstInput.GetHeight() != 0 {
		width = int(firstInput.GetWidth())
		height = int(firstInput.GetHeight())
	}

	// exec
	shell, eol, err := getShellCommand()
	if err != nil {
		return err
	}

	streamerRWC := &OpenPortServerStreamerHelperRWC{
		stream: inputStream,
	}
	commands, _ := shlex.Split(shell)

	ptmx, err := pty.New()
	if err != nil {

		// fallback
		cmd := exec.CommandContext(ctx, commands[0], commands[1:]...)
		stdin, _ := cmd.StdinPipe()
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		cmd.Dir = path
		cmd.Start()

		terminal := term.NewTerminal(streamerRWC, "")
		streamerRWC.sizeCallback = func(width, height int) {
			terminal.SetSize(width, height)
		}
		if width > 0 && height > 0 {
			terminal.SetSize(width, height)
		}

		go io.Copy(terminal, stdout)
		go io.Copy(terminal, stderr)
		for {
			line, err := terminal.ReadLine()
			if errors.Is(err, io.EOF) {
				continue
			}
			if err != nil {
				return err
			}
			if line == "" {
				continue
			}
			stdin.Write([]byte(line + eol))
		}
	} else {
		defer ptmx.Close()
		streamerRWC.sizeCallback = func(width, height int) {
			ptmx.Resize(width, height)
		}
		if width > 0 && height > 0 {
			ptmx.Resize(width, height)
		}

		go io.Copy(ptmx, streamerRWC) // stdin
		go func() {
			if runtime.GOOS == "windows" && strings.Contains(shell, "cmd.exe") {
				// split the first output
				buf := make([]byte, 4096)
				n, err := ptmx.Read(buf)
				if err != nil {
					return
				}
				buf = buf[:n]
				_, after, ok := bytes.Cut(buf, []byte{0x1b, 0x5b, 0x48})
				if ok {
					buf = after
					before, _, ok := bytes.Cut(buf, []byte{0x1b, 0x5d, 0x30})
					if ok {
						buf = before
					}
				}
				streamerRWC.Write(buf)
			}

			io.Copy(streamerRWC, ptmx) // stdout
		}()

		defer func() {
			inputStream.Send(&ypb.Output{
				Control: true,
				Closed:  true,
			})
		}()

		cmd := ptmx.CommandContext(ctx, commands[0], commands[1:]...)
		cmd.Dir = path
		return cmd.Run()
	}
}
