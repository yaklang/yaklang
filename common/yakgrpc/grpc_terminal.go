package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/aymanbagabas/go-pty"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/shlex"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/term"
)

type TerminalWrapper struct {
	current *term.Terminal
}

func (t *TerminalWrapper) Write(p []byte) (n int, err error) {
	return t.current.Write(p)
}

var CtrlCBytes = []byte("^C")

var charsetPriority = map[string]int{
	"C.UTF-8":     2,
	"zh_CN.UTF-8": 1,
}

func getAvailableLocaleUTF8() (string, error) {
	lang := os.Getenv("LANG")
	if lang != "" {
		return lang, nil
	}
	lang = os.Getenv("LC_ALL")
	if lang != "" {
		return lang, nil
	}

	// fallback
	cmd := exec.Command("locale", "-a")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	locales := strings.Split(strings.TrimSpace(string(out)), "\n")
	locales = lo.Filter(locales, func(s string, _ int) bool {
		return strings.HasSuffix(s, ".UTF-8")
	})
	if len(locales) == 0 {
		return "", utils.Errorf("no available locale with UTF-8")
	}

	sort.SliceStable(locales, func(i, j int) bool {
		return charsetPriority[locales[i]] > charsetPriority[locales[j]]
	})

	return locales[0], nil
}

func getShellCommand() (shell, shellOpts, eol string, err error) {
	var (
		finErr               error
		shellNames           []string
		el                   string
		needReplaceBackslash bool
	)

	switch goos := runtime.GOOS; goos {
	case "windows":
		el = "\r\n"
		shellNames = []string{"powershell", "cmd"}
		needReplaceBackslash = true
	case "linux":
		el = "\n"
		shellNames = []string{"bash", "sh"}
		shellOpts = " -i"
	case "darwin":
		el = "\n"
		shellNames = []string{"zsh", "bash", "sh"}
		shellOpts = " -i"
	default:
		return "", "", "", utils.Errorf("unsupported os: %s", goos)
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
		return "", "", "", utils.Errorf("failed to find shell: %s", finErr)
	}
	if needReplaceBackslash {
		// for windows
		shell = strings.ReplaceAll(shell, "\\", "\\\\")
	}
	return shell, shellOpts, el, nil
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
	shell, opts, eol, err := getShellCommand()
	command := shell + opts
	eolBytes := []byte(eol)
	isUnix := runtime.GOOS != "windows"
	envs := os.Environ()
	if isUnix {
		charset, err := getAvailableLocaleUTF8()
		if err == nil {
			for _, key := range []string{"LANG"} {
				envs = append(envs, fmt.Sprintf("%s=%s", key, charset))
			}
		} else {
			log.Warnf("not install available locale with UTF-8: %s", err)
		}
		envs = append(envs, fmt.Sprintf("SHELL=%s", shell))
		envs = append(envs, fmt.Sprintf("TERM=xterm-256color"))
		envs = append(envs, fmt.Sprintf("PATH=%s:%s", consts.GetDefaultYakitEngineDir(), os.Getenv("PATH")))
	} else {
		envs = append(envs, fmt.Sprintf("PATH=%s;%s", consts.GetDefaultYakitEngineDir(), os.Getenv("PATH")))
	}
	envs = append(envs, "TERM_PROGRAM=yaklang")
	baseDir := consts.GetDefaultYakitBaseDir()
	projectDatabaseName := consts.GetDefaultYakitProjectDatabase(baseDir)
	profileDatabaseName := consts.GetDefaultYakitPluginDatabase(baseDir)
	envs = append(envs, fmt.Sprintf("%s=%s", consts.CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME, profileDatabaseName))
	envs = append(envs, fmt.Sprintf("%s=%s", consts.CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, projectDatabaseName))

	if err != nil {
		return err
	}

	streamerRWC := &OpenPortServerStreamerHelperRWC{
		stream: inputStream,
	}
	commands, _ := shlex.Split(command)

	ptmx, err := pty.New()
	if err != nil {

		// fallback
		cmd := exec.CommandContext(ctx, commands[0], commands[1:]...)
		stdin, _ := cmd.StdinPipe()
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		cmd.Dir = path
		if len(envs) > 0 {
			cmd.Env = envs
		}
		cmd.Start()
		// if !isUnix {
		// 	stdin.Write([]byte("chcp 65001"))
		// 	stdin.Write(eolBytes)
		// }
		terminalWrapper := &TerminalWrapper{}

		streamerRWC.sizeCallback = func(width, height int) {
			terminalWrapper.current.SetSize(width, height)
		}
		if width > 0 && height > 0 {
			terminalWrapper.current.SetSize(width, height)
		}

		go io.Copy(terminalWrapper, stdout)
		go io.Copy(terminalWrapper, stderr)

		for {
			terminal := term.NewTerminal(streamerRWC, "")
			terminalWrapper.current = terminal

			for {
				line, err := terminal.ReadLine()
				if errors.Is(err, io.EOF) {
					stdin.Write(eolBytes)
					streamerRWC.Write(CtrlCBytes)
					streamerRWC.Write(eolBytes)
					break
				} else if err != nil {
					return err
				}

				stdin.Write([]byte(line))
				stdin.Write(eolBytes)
			}
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
		if len(envs) > 0 {
			cmd.Env = envs
		}
		cmd.Dir = path
		return cmd.Run()
	}
}
