package bashtools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// filterCommandEcho 过滤命令回显，返回过滤后的输出
func filterCommandEcho(output, command string) string {
	if command == "" {
		return output
	}

	lines := strings.Split(output, "\n")
	var filteredLines []string
	commandFound := false

	for _, line := range lines {
		// 跳过包含命令本身的行（回显）
		if strings.Contains(line, strings.TrimSpace(command)) && !commandFound {
			commandFound = true
			continue
		}
		filteredLines = append(filteredLines, line)
	}

	return strings.Join(filteredLines, "\n")
}

// limitToLastLines 限制输出内容只保留最后N行
func limitToLastLines(output string, maxLines int) string {
	if output == "" {
		return output
	}

	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}

	// 保留最后maxLines行
	lastLines := lines[len(lines)-maxLines:]
	return strings.Join(lastLines, "\n")
}

// BashSession 表示一个bash会话
type BashSession struct {
	Name        string
	ShellType   string
	Cmd         *exec.Cmd
	Pty         *os.File       // PTY主端（仅在非Windows系统使用）
	Stdin       io.WriteCloser // 标准输入（Windows非PTY模式使用）
	Stdout      *bytes.Buffer
	Cancel      context.CancelFunc
	IsRunning   bool
	LastActive  time.Time
	LastCommand string // 记录最后发送的命令，用于过滤回显
	UsePty      bool   // 是否使用PTY模式
	mutex       sync.Mutex
}

type BashSessionContext struct {
	ctx           context.Context
	sessions      map[string]*BashSession
	sessionsMutex sync.RWMutex
}

func NewBashSessionContext(ctx context.Context) *BashSessionContext {
	if ctx == nil {
		ctx = context.Background()
	}

	bashCtx := &BashSessionContext{
		ctx:           ctx,
		sessions:      make(map[string]*BashSession),
		sessionsMutex: sync.RWMutex{},
	}

	// 启动context监控goroutine
	go bashCtx.monitorContext()

	return bashCtx
}

// monitorContext 监控context取消信号，当context被取消时关闭所有会话
func (bashCtx *BashSessionContext) monitorContext() {
	<-bashCtx.ctx.Done()
	log.Infof("BashSessionContext context cancelled, closing all sessions...")
	bashCtx.closeAllSessions()
}

// closeAllSessions 关闭所有会话
func (bashCtx *BashSessionContext) closeAllSessions() {
	bashCtx.sessionsMutex.Lock()
	sessionNames := make([]string, 0, len(bashCtx.sessions))
	for name := range bashCtx.sessions {
		sessionNames = append(sessionNames, name)
	}
	bashCtx.sessionsMutex.Unlock()

	// 并发关闭所有会话
	var wg sync.WaitGroup
	for _, name := range sessionNames {
		wg.Add(1)
		go func(sessionName string) {
			defer wg.Done()
			err := closeSession(bashCtx, sessionName)
			if err != nil {
				log.Errorf("Failed to close session %s: %v", sessionName, err)
			} else {
				log.Debugf("Successfully closed session %s", sessionName)
			}
		}(name)
	}

	// 等待所有会话关闭，但设置超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Infof("All sessions closed successfully")
	case <-time.After(10 * time.Second):
		log.Warnf("Timeout waiting for all sessions to close")
	}
}

// CreateBashTools 创建bash相关的AI工具集合
func CreateBashTools(bashSessionContext *BashSessionContext) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()

	// 注册带会话的bash命令执行工具
	err := factory.RegisterTool(
		"bash_session_execute",
		aitool.WithDescription("一个带会话管理的跨平台TTY输入工具，支持bash、cmd、powershell等多种shell类型，可以创建持久化的shell会话，向TTY会话中写入内容（如命令、交互式输入等），支持超时控制和输出捕获，适用于需要上下文环境的系统管理、自动化脚本执行和运维操作。注意：如果执行结果返回'Input sent, waiting for output...'，说明输入在1秒内没有产生输出，此时可以使用read_bash_session_buffer工具多次读取缓冲区来获取实际输出。"),
		aitool.WithStringParam("input",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("要向TTY会话中写入的内容（可以是命令、文本输入等）"),
		),
		aitool.WithStringParam("session",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("会话名称，用于标识和管理shell会话，不存在时会创建新会话"),
		),
		aitool.WithStringParam("shell",
			aitool.WithParam_Required(false),
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("shell类型: bash, cmd, powershell。如果不指定则自动检测(linux/mac: bash, windows: cmd)"),
		),
		aitool.WithIntegerParam("timeout",
			aitool.WithParam_Required(false),
			aitool.WithParam_Default(10),
			aitool.WithParam_Description("输入超时时间(秒)，默认10秒，不能为0或负数"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			input := params.GetString("input")
			sessionName := params.GetString("session")
			shellType := params.GetString("shell")
			timeoutSeconds := int(params.GetInt("timeout"))

			if input == "" {
				return nil, utils.Errorf("input cannot be empty")
			}
			if sessionName == "" {
				return nil, utils.Errorf("session name cannot be empty")
			}

			// 验证超时参数
			if timeoutSeconds <= 0 {
				timeoutSeconds = 10
			}

			// 自动检测shell类型
			if shellType == "" {
				switch runtime.GOOS {
				case "windows":
					shellType = "cmd"
				case "linux", "darwin":
					shellType = "bash"
				default:
					shellType = "bash"
				}
			}

			// 执行带会话的输入
			result, err := executeSessionCommand(bashSessionContext, sessionName, input, shellType, timeoutSeconds, stdout, stderr)
			if err != nil {
				return nil, err
			}

			return result, nil
		}),
	)
	if err != nil {
		log.Errorf("register bash_session_execute tool: %v", err)
		return nil, err
	}

	// 注册列出会话工具
	err = factory.RegisterTool(
		"list_bash_sessions",
		aitool.WithDescription("列出当前所有的bash会话及其状态信息"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			bashSessionContext.sessionsMutex.RLock()
			defer bashSessionContext.sessionsMutex.RUnlock()

			sessionsInfo := make([]map[string]interface{}, 0, len(bashSessionContext.sessions))
			for _, session := range bashSessionContext.sessions {
				session.mutex.Lock()
				info := map[string]interface{}{
					"name":        session.Name,
					"shell_type":  session.ShellType,
					"is_running":  session.IsRunning,
					"last_active": session.LastActive.Format("2006-01-02 15:04:05"),
				}
				session.mutex.Unlock()
				sessionsInfo = append(sessionsInfo, info)
			}

			return map[string]interface{}{
				"total_sessions": len(bashSessionContext.sessions),
				"sessions":       sessionsInfo,
			}, nil
		}),
	)
	if err != nil {
		log.Errorf("register list_bash_sessions tool: %v", err)
	}

	// 注册关闭会话工具
	err = factory.RegisterTool(
		"close_bash_session",
		aitool.WithDescription("关闭指定的bash会话"),
		aitool.WithStringParam("session",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("要关闭的会话名称"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			sessionName := params.GetString("session")
			if sessionName == "" {
				return nil, utils.Errorf("session name cannot be empty")
			}

			err := closeSession(bashSessionContext, sessionName)
			if err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"session": sessionName,
				"status":  "closed",
			}, nil
		}),
	)
	if err != nil {
		log.Errorf("register close_bash_session tool: %v", err)
	}

	// 注册读取buffer工具
	err = factory.RegisterTool(
		"read_bash_session_buffer",
		aitool.WithDescription("读取指定bash会话的buffer内容，可以查看当前缓冲区中的所有输出。特别适用于当bash_session_execute返回'Input sent, waiting for output...'时，通过多次调用此工具来获取TTY会话的实际输出结果。支持限制返回行数以控制输出量。"),
		aitool.WithStringParam("session",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("要读取buffer的会话名称"),
		),
		aitool.WithIntegerParam("lines",
			aitool.WithParam_Required(false),
			aitool.WithParam_Default(0),
			aitool.WithParam_Description("限制返回的行数，0表示返回所有内容，正数表示返回最后N行"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			sessionName := params.GetString("session")
			maxLines := int(params.GetInt("lines"))

			if sessionName == "" {
				return nil, utils.Errorf("session name cannot be empty")
			}

			// 获取会话
			bashSessionContext.sessionsMutex.RLock()
			session, exists := bashSessionContext.sessions[sessionName]
			bashSessionContext.sessionsMutex.RUnlock()

			if !exists {
				return nil, utils.Errorf("session %s not found", sessionName)
			}

			// 读取buffer内容
			session.mutex.Lock()
			bufferContent := toUTF8(session.Stdout.Bytes())
			session.mutex.Unlock()

			// 如果指定了行数限制
			if maxLines > 0 {
				bufferContent = limitToLastLines(bufferContent, maxLines)
			}

			// 将内容写入到stdout
			if stdout != nil && len(bufferContent) > 0 {
				stdout.Write([]byte(fmt.Sprintf("Buffer content for session '%s':\n%s\n", sessionName, bufferContent)))
			}

			return map[string]interface{}{
				"session":        sessionName,
				"buffer_content": bufferContent,
				"buffer_size":    len(bufferContent),
				"lines_count":    len(strings.Split(bufferContent, "\n")),
			}, nil
		}),
	)
	if err != nil {
		log.Errorf("register read_bash_session_buffer tool: %v", err)
	}

	return factory.Tools(), nil
}

// executeSessionCommand 向指定会话中写入输入内容
func executeSessionCommand(bashSessionContext *BashSessionContext, sessionName, input, shellType string, timeoutSeconds int, stdout, stderr io.Writer) (string, error) {
	session, err := getOrCreateSession(bashSessionContext, sessionName, shellType)
	if err != nil {
		return "", err
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// 更新最后活跃时间和输入记录
	session.LastActive = time.Now()
	session.LastCommand = input

	// 如果会话已经结束，重新创建
	if !session.IsRunning {
		err := createSessionProcess(bashSessionContext, session)
		if err != nil {
			return "", utils.Errorf("failed to restart session %s: %v", session.Name, err)
		}
	}

	// 向会话写入内容
	var writeErr error
	if session.UsePty {
		// PTY 模式：写入到 PTY
		_, writeErr = session.Pty.Write([]byte(input + "\n"))
	} else {
		// 非 PTY 模式：写入到 Stdin
		_, writeErr = session.Stdin.Write([]byte(input + "\n"))
	}

	if writeErr != nil {
		// 如果写入失败，尝试重新创建会话
		err2 := createSessionProcess(bashSessionContext, session)
		if err2 != nil {
			return "", utils.Errorf("failed to send input and restart session %s: %v, %v", session.Name, writeErr, err2)
		}

		// 重试写入
		if session.UsePty {
			_, writeErr = session.Pty.Write([]byte(input + "\n"))
		} else {
			_, writeErr = session.Stdin.Write([]byte(input + "\n"))
		}

		if writeErr != nil {
			return "", utils.Errorf("failed to send input to session %s: %v", session.Name, writeErr)
		}
	}

	// 等待输出，带超时控制
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		time.Sleep(1 * time.Second)
		stdoutStr := session.Stdout.Bytes()

		session.Stdout.Reset()
		// 过滤命令回显
		filteredStdout := filterCommandEcho(string(stdoutStr), session.LastCommand)

		// 限制输出内容只保留最后10行
		limitedStdout := limitToLastLines(filteredStdout, 10)

		// 将输出写入到提供的writer中
		if stdout != nil && len(limitedStdout) > 0 {
			stdout.Write([]byte(fmt.Sprintf("Stdout:\n%s\n", limitedStdout)))
		}

		// 返回结果
		if len(limitedStdout) > 0 {
			resultChan <- limitedStdout
		} else {
			resultChan <- "Input sent, waiting for output..."
		}
	}()

	// 等待结果或超时
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return "", err
	case <-ctx.Done():
		// 超时时返回当前缓冲区内容
		stdoutStr := toUTF8(session.Stdout.Bytes())
		session.Stdout.Reset()
		// 过滤命令回显
		filteredStdout := filterCommandEcho(stdoutStr, session.LastCommand)

		// 限制输出内容只保留最后10行
		limitedStdout := limitToLastLines(filteredStdout, 10)

		if len(limitedStdout) > 0 {
			result := fmt.Sprintf("Input timed out after %d seconds. Current output:\nStdout: %s",
				timeoutSeconds, limitedStdout)
			return result, nil
		}
		return fmt.Sprintf("Input timed out after %d seconds with no output", timeoutSeconds), nil
	}
}

// getOrCreateSession 获取或创建会话
func getOrCreateSession(bashSessionContext *BashSessionContext, sessionName, shellType string) (*BashSession, error) {
	bashSessionContext.sessionsMutex.Lock()
	defer bashSessionContext.sessionsMutex.Unlock()

	// 检查会话是否已存在
	if session, exists := bashSessionContext.sessions[sessionName]; exists {
		return session, nil
	}

	// 自动检测shell类型
	if shellType == "" {
		switch runtime.GOOS {
		case "windows":
			shellType = "cmd"
		case "linux", "darwin":
			shellType = "bash"
		default:
			shellType = "bash"
		}
	}

	// 创建新会话
	session := &BashSession{
		Name:       sessionName,
		ShellType:  shellType,
		Stdout:     &bytes.Buffer{},
		LastActive: time.Now(),
		IsRunning:  false,
	}

	// 创建会话进程
	err := createSessionProcess(bashSessionContext, session)
	if err != nil {
		return nil, err
	}

	bashSessionContext.sessions[sessionName] = session
	return session, nil
}

// createSessionProcess 创建会话进程
func createSessionProcess(bashSessionContext *BashSessionContext, session *BashSession) error {
	// 如果有旧进程在运行，先清理
	if session.Cmd != nil && session.Cancel != nil {
		session.Cancel()
		session.Cmd.Wait()
	}

	// 如果有旧的 PTY，先关闭
	if session.Pty != nil {
		session.Pty.Close()
	}

	// 如果有旧的 Stdin，先关闭
	if session.Stdin != nil {
		session.Stdin.Close()
	}

	// 创建新的上下文，继承自BashSessionContext的context
	ctx, cancel := context.WithCancel(bashSessionContext.ctx)
	session.Cancel = cancel

	// 根据shell类型创建命令
	var cmd *exec.Cmd
	switch session.ShellType {
	case "bash":
		cmd = exec.CommandContext(ctx, "bash")
	case "cmd":
		cmd = exec.CommandContext(ctx, "cmd")
	case "powershell":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NoExit")
	default:
		return utils.Errorf("unsupported shell type: %s", session.ShellType)
	}

	// 决定是否使用 PTY 模式
	session.UsePty = runtime.GOOS != "windows"

	if session.UsePty {
		// 非 Windows 系统：使用 PTY 模式
		ptyFile, err := pty.Start(cmd)
		if err != nil {
			return utils.Errorf("failed to start %s process with pty: %v", session.ShellType, err)
		}

		session.Cmd = cmd
		session.Pty = ptyFile
		session.IsRunning = true

		// 启动输出读取goroutine，从 pty 读取所有输出（stdout + stderr）
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Debugf("PTY copy goroutine recovered from panic: %v", r)
				}
			}()

			// 创建一个混合缓冲区来同时捕获 stdout 和 stderr
			mixedOutput := &bytes.Buffer{}

			// 使用 TeeReader 同时写入到 session.Stdout 和 mixedOutput
			teeReader := io.TeeReader(session.Pty, io.MultiWriter(session.Stdout, mixedOutput))

			// 读取所有输出
			_, err := io.Copy(io.Discard, teeReader)
			if err != nil && err != io.EOF {
				log.Debugf("PTY copy finished with error: %v", err)
			}
		}()
	} else {
		// Windows 系统：使用直接管道模式
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return utils.Errorf("failed to create stdin pipe: %v", err)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			stdin.Close()
			return utils.Errorf("failed to create stdout pipe: %v", err)
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			stdin.Close()
			stdout.Close()
			return utils.Errorf("failed to create stderr pipe: %v", err)
		}

		// 启动进程
		err = cmd.Start()
		if err != nil {
			stdin.Close()
			stdout.Close()
			stderr.Close()
			return utils.Errorf("failed to start %s process: %v", session.ShellType, err)
		}

		session.Cmd = cmd
		session.Stdin = stdin
		session.IsRunning = true

		// 启动输出读取goroutine，分别处理 stdout 和 stderr
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Debugf("Stdout copy goroutine recovered from panic: %v", r)
				}
			}()

			// 读取 stdout
			_, err := io.Copy(session.Stdout, stdout)
			if err != nil && err != io.EOF {
				log.Debugf("Stdout copy finished with error: %v", err)
			}
		}()

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Debugf("Stderr copy goroutine recovered from panic: %v", r)
				}
			}()

			// 读取 stderr
			_, err := io.Copy(session.Stdout, stderr)
			if err != nil && err != io.EOF {
				log.Debugf("Stderr copy finished with error: %v", err)
			}
		}()
	}

	// 监控进程状态
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Debugf("Process monitor goroutine recovered from panic: %v", r)
			}
		}()

		err := cmd.Wait()
		if err != nil && !strings.Contains(err.Error(), "signal: killed") {
			log.Debugf("Process %s exited with error: %v", session.Name, err)
		}

		session.mutex.Lock()
		session.IsRunning = false
		session.mutex.Unlock()
	}()

	return nil
}

// closeSession 关闭指定会话
func closeSession(bashSessionContext *BashSessionContext, sessionName string) error {
	// 首先获取会话引用，避免长时间持有全局锁
	bashSessionContext.sessionsMutex.RLock()
	session, exists := bashSessionContext.sessions[sessionName]
	bashSessionContext.sessionsMutex.RUnlock()

	if !exists {
		return utils.Errorf("session %s not found", sessionName)
	}

	// 获取会话锁并检查状态
	session.mutex.Lock()

	// 如果会话已经关闭，直接返回
	if !session.IsRunning {
		session.mutex.Unlock()
		// 从全局map中删除
		bashSessionContext.sessionsMutex.Lock()
		delete(bashSessionContext.sessions, sessionName)
		bashSessionContext.sessionsMutex.Unlock()
		return nil
	}

	// 关闭进程的正确顺序：
	// 1. 首先关闭输入/输出，让进程知道没有更多输入
	if session.UsePty {
		// PTY 模式：关闭 PTY
		if session.Pty != nil {
			session.Pty.Close()
			session.Pty = nil
		}
	} else {
		// 非 PTY 模式：关闭 Stdin
		if session.Stdin != nil {
			session.Stdin.Close()
			session.Stdin = nil
		}
	}

	// 2. 发送取消信号
	if session.Cancel != nil {
		session.Cancel()
	}

	// 3. 等待进程退出，但加上超时保护
	if session.Cmd != nil && session.Cmd.Process != nil {
		// 检查进程是否已经退出
		if session.Cmd.ProcessState != nil && session.Cmd.ProcessState.Exited() {
			// 进程已经退出，无需等待
			log.Debugf("Session %s process already exited", sessionName)
		} else {
			// 创建一个带超时的等待
			done := make(chan error, 1)
			go func() {
				done <- session.Cmd.Wait()
			}()

			// 等待进程退出，最多等待2秒
			select {
			case <-done:
				// 进程正常退出
				log.Debugf("Session %s exited gracefully", sessionName)
			case <-time.After(2 * time.Second):
				// 超时，尝试强制杀死进程
				log.Debugf("Session %s did not exit gracefully, attempting to kill process", sessionName)
				if session.Cmd.Process != nil {
					// 尝试SIGTERM然后SIGKILL
					if runtime.GOOS != "windows" {
						// Unix-like系统：先发送TERM信号
						session.Cmd.Process.Signal(os.Interrupt)
						select {
						case <-done:
							log.Debugf("Session %s terminated by SIGTERM", sessionName)
						case <-time.After(500 * time.Millisecond):
							// SIGTERM无效，使用SIGKILL
							session.Cmd.Process.Kill()
							select {
							case <-done:
								log.Debugf("Session %s killed by SIGKILL", sessionName)
							case <-time.After(500 * time.Millisecond):
								// 即使kill失败也继续，可能是僵尸进程
								log.Debugf("Session %s process may be zombie, continuing cleanup", sessionName)
							}
						}
					} else {
						// Windows系统：直接kill
						session.Cmd.Process.Kill()
						select {
						case <-done:
							log.Debugf("Session %s killed on Windows", sessionName)
						case <-time.After(500 * time.Millisecond):
							log.Debugf("Session %s kill timeout on Windows, continuing cleanup", sessionName)
						}
					}
				}
			}
		}
	}

	// 4. PTY 已经在步骤1中关闭，无需额外关闭管道

	session.IsRunning = false
	session.mutex.Unlock()

	// 从会话列表中删除（需要重新获取全局锁）
	bashSessionContext.sessionsMutex.Lock()
	delete(bashSessionContext.sessions, sessionName)
	bashSessionContext.sessionsMutex.Unlock()

	return nil
}

// toUTF8 将可能包含GBK编码的字符串转换为UTF-8
func toUTF8(s []byte) string {
	// 尝试GBK解码（主要针对Windows）
	if result, err := codec.GBKSafeString(s); err == nil {
		return result
	}
	// 如果GBK解码失败，返回原字符串
	return string(s)
}
