package localmodel

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ProcessInfo 进程信息结构
type ProcessInfo struct {
	PID     int
	PPID    int
	Command string
	Args    []string
	WorkDir string
}

// findLlamaServerProcesses 查找所有 llama-server 进程
func (m *Manager) findLlamaServerProcesses() ([]*ProcessInfo, error) {
	switch runtime.GOOS {
	case "windows":
		return m.findProcessesWindows()
	case "darwin", "linux":
		return m.findProcessesUnix()
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// findProcessesWindows Windows 系统进程发现
func (m *Manager) findProcessesWindows() ([]*ProcessInfo, error) {
	// 使用 wmic 命令获取进程信息
	cmd := exec.Command("wmic", "process", "where", "name='llama-server.exe'", "get", "ProcessId,ParentProcessId,CommandLine", "/format:csv")
	output, err := cmd.Output()
	if err != nil {
		// 如果 wmic 失败，尝试使用 tasklist
		return m.findProcessesWindowsTasklist()
	}
	return m.parseWmicOutput(toUTF8(output))
}

// findProcessesWindowsTasklist Windows tasklist 备用方案
func (m *Manager) findProcessesWindowsTasklist() ([]*ProcessInfo, error) {
	cmd := exec.Command("tasklist", "/fo", "csv", "/v")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute tasklist: %v", err)
	}

	return m.parseTasklistOutput(toUTF8(output))
}

// findProcessesUnix Unix 系统（Linux/macOS）进程发现
func (m *Manager) findProcessesUnix() ([]*ProcessInfo, error) {
	// 使用 ps 命令获取详细进程信息
	cmd := exec.Command("ps", "axo", "pid,ppid,command")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ps command: %v", err)
	}

	return m.parsePsOutput(toUTF8(output))
}

// parseWmicOutput 解析 wmic 命令输出
func (m *Manager) parseWmicOutput(output string) ([]*ProcessInfo, error) {
	var processes []*ProcessInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Node,") {
			continue
		}

		// CSV 格式解析
		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			continue
		}

		commandLine := parts[1]
		if !m.isLlamaServerCommand(commandLine) {
			continue
		}

		ppidStr := parts[2]
		pidStr := parts[3]

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		ppid, err := strconv.Atoi(ppidStr)
		if err != nil {
			ppid = 0
		}

		args := strings.Fields(commandLine)
		processes = append(processes, &ProcessInfo{
			PID:     pid,
			PPID:    ppid,
			Command: commandLine,
			Args:    args,
		})
	}

	return processes, nil
}

// parseTasklistOutput 解析 tasklist 命令输出
func (m *Manager) parseTasklistOutput(output string) ([]*ProcessInfo, error) {
	var processes []*ProcessInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "\"Image Name\"") {
			continue
		}

		// 简单的 CSV 解析，查找包含 llama-server 的进程
		if strings.Contains(line, "llama-server") {
			// 从 tasklist 输出中提取 PID
			re := regexp.MustCompile(`"(\d+)"`)
			matches := re.FindAllStringSubmatch(line, -1)
			if len(matches) >= 2 {
				pidStr := matches[1][1] // 第二个数字通常是 PID
				pid, err := strconv.Atoi(pidStr)
				if err == nil {
					processes = append(processes, &ProcessInfo{
						PID:     pid,
						Command: line,
						Args:    []string{"llama-server"}, // 简化处理
					})
				}
			}
		}
	}

	return processes, nil
}

// parsePsOutput 解析 ps 命令输出
func (m *Manager) parsePsOutput(output string) ([]*ProcessInfo, error) {
	var processes []*ProcessInfo
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		if i == 0 { // 跳过头部
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// ps 输出格式: PID PPID COMMAND
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		pidStr := parts[0]
		ppidStr := parts[1]
		command := strings.Join(parts[2:], " ")

		if !m.isLlamaServerCommand(command) {
			continue
		}

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		ppid, err := strconv.Atoi(ppidStr)
		if err != nil {
			ppid = 0
		}

		args := strings.Fields(command)
		processes = append(processes, &ProcessInfo{
			PID:     pid,
			PPID:    ppid,
			Command: command,
			Args:    args,
		})
	}

	return processes, nil
}

// isLlamaServerCommand 检查是否是 llama-server 命令
func (m *Manager) isLlamaServerCommand(command string) bool {
	// 检查命令是否包含 llama-server
	return strings.Contains(command, "llama-server")
}

// findLlamaServerProcessesWindows 在Windows下查找匹配host和port的llama-server进程
func (m *Manager) findLlamaServerProcessesWindows(host string, port int32) ([]int, error) {
	var pids []int

	// 使用wmic查找llama-server进程
	cmd := exec.Command("wmic", "process", "where", "name='llama-server.exe'", "get", "ProcessId,CommandLine", "/format:csv")
	output, err := cmd.Output()
	if err != nil {
		// 如果wmic失败，尝试使用tasklist
		return m.findLlamaServerProcessesWindowsTasklist(host, port)
	}

	lines := strings.Split(toUTF8(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Node,") {
			continue
		}

		// CSV格式: Node,CommandLine,ProcessId
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}

		commandLine := parts[1]
		pidStr := parts[2]

		// 检查命令行是否包含匹配的host和port
		if m.isLlamaServerCommandMatching(commandLine, host, port) {
			if pid, err := strconv.Atoi(pidStr); err == nil {
				pids = append(pids, pid)
				log.Infof("Found matching llama-server process: PID=%d, Command=%s", pid, commandLine)
			}
		}
	}

	return pids, nil
}

// findLlamaServerProcessesWindowsTasklist 使用tasklist作为备用方案
func (m *Manager) findLlamaServerProcessesWindowsTasklist(host string, port int32) ([]int, error) {
	var pids []int

	cmd := exec.Command("tasklist", "/fo", "csv", "/v")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute tasklist: %v", err)
	}

	lines := strings.Split(toUTF8(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "\"Image Name\"") {
			continue
		}

		// 查找包含llama-server的进程
		if strings.Contains(line, "llama-server") {
			// 这是一个简化版本，实际实现中可能需要更复杂的解析
			// 因为tasklist不直接提供命令行参数
			re := regexp.MustCompile(`"(\d+)"`)
			matches := re.FindAllStringSubmatch(line, -1)
			if len(matches) >= 2 {
				pidStr := matches[1][1] // 第二个数字通常是PID
				if pid, err := strconv.Atoi(pidStr); err == nil {
					pids = append(pids, pid)
					log.Infof("Found llama-server process (via tasklist): PID=%d", pid)
				}
			}
		}
	}

	return pids, nil
}

// isLlamaServerCommandMatching 检查llama-server命令行是否匹配指定的host和port
func (m *Manager) isLlamaServerCommandMatching(commandLine string, host string, port int32) bool {
	// 检查命令行是否包含指定的host和port参数
	hostPattern := fmt.Sprintf("--host %s", host)
	portPattern := fmt.Sprintf("--port %d", port)

	return strings.Contains(commandLine, "llama-server") &&
		strings.Contains(commandLine, hostPattern) &&
		strings.Contains(commandLine, portPattern)
}

// toUTF8 转换字节数据为UTF-8字符串
func toUTF8(data []byte) string {
	// 先尝试判断是否是 UTF-8
	if utf8.Valid(data) {
		return string(data)
	}
	// 否则按 GBK 转 UTF-8
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	utf8Data, _ := io.ReadAll(reader)
	return string(utf8Data)
}
