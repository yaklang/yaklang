package ffmpegutils

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
)

// DisplayInfo 表示显示器信息
type DisplayInfo struct {
	ID      int    // 显示器ID
	X       int    // X偏移
	Y       int    // Y偏移
	Width   int    // 宽度
	Height  int    // 高度
	Name    string // 显示器名称
	Primary bool   // 是否为主显示器
}

// ExtractUserScreenShot 捕获用户屏幕截图，支持多屏幕处理
func ExtractUserScreenShot(opts ...Option) (*FfmpegStreamResult, error) {
	if ffmpegBinaryPath == "" {
		return nil, fmt.Errorf("ffmpeg binary path is not configured")
	}

	o := newDefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	// 如果没有设置context，使用默认的
	if o.ctx == nil {
		o.ctx = context.Background()
	}

	// 根据操作系统平台选择最佳的截图方法
	switch runtime.GOOS {
	case "darwin":
		return captureScreenMacOS(o)
	case "windows":
		return captureScreenWindows(o)
	case "linux":
		return captureScreenLinux(o)
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// captureScreenMacOS 在 macOS 上截图
func captureScreenMacOS(o *options) (*FfmpegStreamResult, error) {
	// 创建临时文件
	outputPath := consts.TempAIFileFast(
		fmt.Sprintf("ffmpeg-screenshot-%v-%v.jpeg", utils.DatetimePretty2(), "*"),
	)
	defer os.Remove(outputPath) // 确保在函数结束时清理

	// 检测显示器数量和信息
	displays := detectDisplaysMacOS()
	if len(displays) == 0 {
		log.Infof("failed to detect displays, using single screen capture")
		// 如果检测失败，回退到单屏幕模式
		return captureSingleScreenMacOS(outputPath, o)
	}

	if len(displays) <= 1 {
		if len(displays) == 1 && displays[0].ID == 0 {
			// 只有一个屏幕，直接截图
			return captureSingleScreenMacOS(outputPath, o)
		}
	}

	// 多屏幕处理
	log.Infof("detected %d displays on macOS", len(displays))
	return captureMultipleScreensMacOS(displays, outputPath, o)
}

// captureScreenWindows 在 Windows 上截图
func captureScreenWindows(o *options) (*FfmpegStreamResult, error) {
	// 创建临时文件
	tmpFile, err := ioutil.TempFile(consts.GetDefaultYakitBaseTempDir(), "screenshot-*.png")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpFile.Close()
	outputPath := tmpFile.Name()
	defer os.Remove(outputPath)

	// 检测显示器信息
	displays, err := detectDisplaysWindows()
	if err != nil {
		log.Infof("failed to detect displays, using single screen capture: %v", err)
		return captureSingleScreenWindows(outputPath, o)
	}

	if len(displays) <= 1 {
		return captureSingleScreenWindows(outputPath, o)
	}

	// 多屏幕处理
	log.Infof("detected %d displays on Windows", len(displays))
	return captureMultipleScreensWindows(displays, outputPath, o)
}

// captureScreenLinux 在 Linux 上截图
func captureScreenLinux(o *options) (*FfmpegStreamResult, error) {
	// 创建临时文件
	tmpFile, err := ioutil.TempFile(consts.GetDefaultYakitBaseTempDir(), "screenshot-*.png")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpFile.Close()
	outputPath := tmpFile.Name()
	defer os.Remove(outputPath)

	// Linux 上使用 x11grab
	displays, err := detectDisplaysLinux()
	if err != nil {
		log.Infof("failed to detect displays, using single screen capture: %v", err)
		return captureSingleScreenLinux(outputPath, o)
	}

	if len(displays) <= 1 {
		return captureSingleScreenLinux(outputPath, o)
	}

	// 多屏幕处理
	log.Infof("detected %d displays on Linux", len(displays))
	return captureMultipleScreensLinux(displays, outputPath, o)
}

// captureSingleScreenMacOS 单屏幕截图 (macOS)
func captureSingleScreenMacOS(outputPath string, o *options) (*FfmpegStreamResult, error) {
	args := []string{
		"-f", "avfoundation",
		"-capture_cursor", "1", // 捕获鼠标光标
		"-capture_mouse_clicks", "0", // 不捕获鼠标点击
		"-i", "0:", // 截取屏幕0，macOS avfoundation 从0开始
		"-vframes", "1", // 只捕获一帧
		"-pix_fmt", "rgba", // 使用 RGBA 像素格式保持透明度和质量
		"-compression_level", "0", // PNG 无损压缩
		"-pred", "1", // PNG 预测滤波器
		"-q:v", strconv.Itoa(o.frameQuality), // 使用配置的质量参数
		"-y", // 覆盖输出文件
		outputPath,
	}

	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Infof("executing ffmpeg screenshot command: %s", cmd.String())
	}

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg screenshot failed: %w", err)
	}

	return readScreenshotResult(outputPath)
}

// captureSingleScreenWindows 单屏幕截图 (Windows)
func captureSingleScreenWindows(outputPath string, o *options) (*FfmpegStreamResult, error) {
	args := []string{
		"-f", "gdigrab",
		"-draw_mouse", "1", // 捕获鼠标光标
		"-i", "desktop", // 截取桌面
		"-vframes", "1", // 只捕获一帧
		"-pix_fmt", "rgba", // 使用 RGBA 像素格式保持质量
		"-compression_level", "0", // PNG 无损压缩
		"-pred", "1", // PNG 预测滤波器
		"-q:v", strconv.Itoa(o.frameQuality), // 使用配置的质量参数
		"-y", // 覆盖输出文件
		outputPath,
	}

	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Infof("executing ffmpeg screenshot command: %s", cmd.String())
	}

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg screenshot failed: %w", err)
	}

	return readScreenshotResult(outputPath)
}

// captureSingleScreenLinux 单屏幕截图 (Linux)
func captureSingleScreenLinux(outputPath string, o *options) (*FfmpegStreamResult, error) {
	args := []string{
		"-f", "x11grab",
		"-draw_mouse", "1", // 捕获鼠标光标
		"-i", ":0.0", // 截取 display :0.0
		"-vframes", "1", // 只捕获一帧
		"-pix_fmt", "rgba", // 使用 RGBA 像素格式保持质量
		"-compression_level", "0", // PNG 无损压缩
		"-pred", "1", // PNG 预测滤波器
		"-q:v", strconv.Itoa(o.frameQuality), // 使用配置的质量参数
		"-y", // 覆盖输出文件
		outputPath,
	}

	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Infof("executing ffmpeg screenshot command: %s", cmd.String())
	}

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg screenshot failed: %w", err)
	}

	return readScreenshotResult(outputPath)
}

// readScreenshotResult 读取截图结果
func readScreenshotResult(outputPath string) (*FfmpegStreamResult, error) {
	if !utils.FileExists(outputPath) {
		return nil, fmt.Errorf("screenshot file not created: %s", outputPath)
	}

	data, err := ioutil.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read screenshot file: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("screenshot file is empty")
	}

	mimeObj := mimetype.Detect(data)
	result := &FfmpegStreamResult{
		RawData:   data,
		Timestamp: 0, // 截图没有时间戳概念
		Error:     nil,
	}
	if mimeObj != nil {
		result.MIMEType = mimeObj.String()
		result.MIMETypeObj = mimeObj
	}

	log.Infof("successfully captured screenshot, size: %d bytes, type: %s", len(data), mimeObj.String())
	return result, nil
}

// detectDisplaysMacOS 检测 macOS 上的显示器
func detectDisplaysMacOS() []DisplayInfo {
	// 使用 ffmpeg 检测显示器
	return detectDisplaysMacOSSimple()
}

// detectDisplaysMacOSSimple 简单检测 macOS 显示器
func detectDisplaysMacOSSimple() []DisplayInfo {
	// 使用 ffmpeg 列出 avfoundation 设备
	cmd := exec.Command(ffmpegBinaryPath, "-f", "avfoundation", "-list_devices", "true", "-i", "dummy")
	output, _ := cmd.CombinedOutput()
	// 注意：即使命令失败，也可能输出设备信息，所以我们继续解析输出

	// 使用正则表达式解析输出，提取屏幕设备索引
	// 匹配格式：[数字] Capture screen 数字
	outputStr := string(output)
	displays := []DisplayInfo{}

	// 正则表达式：匹配 [数字] Capture screen 数字
	re := regexp.MustCompile(`(?i)\[(\d+)\] Capture screen (\d+)`)
	matches := re.FindAllStringSubmatch(outputStr, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			// match[1] 是 [] 中的索引，这是我们要用的设备ID
			if deviceID, err := strconv.Atoi(match[1]); err == nil {
				// match[2] 是 "Capture screen" 后面的数字，仅用于显示名称
				screenNum := match[2]

				displays = append(displays, DisplayInfo{
					ID:      deviceID,        // 使用 [] 中的索引作为设备ID
					X:       deviceID * 1920, // 假设水平排列
					Y:       0,
					Width:   1920, // 默认分辨率，实际使用时 ffmpeg 会自动检测
					Height:  1080,
					Name:    fmt.Sprintf("Capture screen %s", screenNum),
					Primary: deviceID == 0, // ID为0的是主屏幕
				})

				log.Infof("found screen device [%d] for screen %s", deviceID, screenNum)
			}
		}
	}

	if len(displays) == 0 {
		log.Infof("no screen devices found in ffmpeg output, using default display")
		// 如果没有检测到，返回默认屏幕
		displays = append(displays, DisplayInfo{ID: 0, X: 0, Y: 0, Width: 1920, Height: 1080, Name: "Main Display", Primary: true})
	}

	return displays
}

// detectDisplaysWindows 检测 Windows 上的显示器
func detectDisplaysWindows() ([]DisplayInfo, error) {
	// 在 Windows 上，可以尝试使用 WMI 查询或直接使用 ffmpeg 检测
	return detectDisplaysWindowsSimple()
}

// detectDisplaysWindowsSimple 简单检测 Windows 显示器
func detectDisplaysWindowsSimple() ([]DisplayInfo, error) {
	// 尝试使用 PowerShell 获取显示器信息
	cmd := exec.Command("powershell", "-Command", "Get-WmiObject -Class Win32_VideoController | Select-Object Name, VideoModeDescription")
	output, err := cmd.Output()
	if err != nil {
		// 如果失败，返回默认屏幕
		return []DisplayInfo{{ID: 0, X: 0, Y: 0, Width: 1920, Height: 1080, Name: "Primary Display", Primary: true}}, nil
	}

	outputStr := string(output)
	displays := []DisplayInfo{}

	// 解析 PowerShell 输出
	lines := strings.Split(outputStr, "\n")
	displayCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "x") && (strings.Contains(line, "1920") || strings.Contains(line, "1366") || strings.Contains(line, "2560")) {
			displayCount++
			displays = append(displays, DisplayInfo{
				ID:      displayCount - 1,          // Windows 从 0 开始
				X:       (displayCount - 1) * 1920, // 假设水平排列
				Y:       0,
				Width:   1920,
				Height:  1080,
				Name:    fmt.Sprintf("Display %d", displayCount),
				Primary: displayCount == 1,
			})
		}
	}

	if len(displays) == 0 {
		displays = append(displays, DisplayInfo{ID: 0, X: 0, Y: 0, Width: 1920, Height: 1080, Name: "Primary Display", Primary: true})
	}

	return displays, nil
}

// detectDisplaysLinux 检测 Linux 上的显示器
func detectDisplaysLinux() ([]DisplayInfo, error) {
	// 使用 xrandr 命令检测显示器
	cmd := exec.Command("xrandr", "--query")
	output, err := cmd.Output()
	if err != nil {
		// 如果 xrandr 失败，返回默认屏幕
		return []DisplayInfo{{ID: 0, X: 0, Y: 0, Width: 1920, Height: 1080, Name: "Display :0.0", Primary: true}}, nil
	}

	displays := []DisplayInfo{}
	lines := strings.Split(string(output), "\n")

	for i, line := range lines {
		// 查找连接的显示器行，例如："DP-2 connected 1920x1080+1920+0"
		if strings.Contains(line, " connected ") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				name := parts[0]
				resolution := parts[2]

				// 解析分辨率和位置信息，例如 "1920x1080+1920+0"
				var width, height, x, y int
				if n, err := fmt.Sscanf(resolution, "%dx%d+%d+%d", &width, &height, &x, &y); n == 4 && err == nil {
					displays = append(displays, DisplayInfo{
						ID:      i,
						X:       x,
						Y:       y,
						Width:   width,
						Height:  height,
						Name:    name,
						Primary: strings.Contains(line, "primary"),
					})
				}
			}
		}
	}

	if len(displays) == 0 {
		displays = append(displays, DisplayInfo{ID: 0, X: 0, Y: 0, Width: 1920, Height: 1080, Name: "Display :0.0", Primary: true})
	}

	return displays, nil
}

// captureMultipleScreensMacOS 多屏幕截图并拼接 (macOS)
func captureMultipleScreensMacOS(displays []DisplayInfo, outputPath string, o *options) (*FfmpegStreamResult, error) {
	// 创建临时目录存储单个屏幕截图
	tempDir, err := ioutil.TempDir(consts.GetDefaultYakitBaseTempDir(), "multi-screen-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// 捕获每个屏幕
	var screenFiles []string
	for i, display := range displays {
		screenFile := fmt.Sprintf("%s/screen_%d.png", tempDir, i)

		args := []string{
			"-f", "avfoundation",
			"-i", strconv.Itoa(display.ID) + ":", // 添加冒号表示只要视频不要音频
			"-vframes", "1",
			"-y",
			screenFile,
		}

		cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
		if o.debug {
			cmd.Stderr = log.NewLogWriter(log.DebugLevel)
			log.Infof("capturing screen %d: %s", display.ID, cmd.String())
		}

		err := cmd.Run()
		if err != nil {
			log.Infof("failed to capture screen %d: %v", display.ID, err)
			continue
		}

		if utils.FileExists(screenFile) {
			screenFiles = append(screenFiles, screenFile)
		}
	}

	if len(screenFiles) == 0 {
		return nil, fmt.Errorf("failed to capture any screens")
	}

	// 拼接屏幕
	return concatenateScreens(screenFiles, outputPath, o)
}

// captureMultipleScreensWindows 多屏幕截图并拼接 (Windows)
func captureMultipleScreensWindows(displays []DisplayInfo, outputPath string, o *options) (*FfmpegStreamResult, error) {
	if len(displays) <= 1 {
		return captureSingleScreenWindows(outputPath, o)
	}

	// 对于 Windows，我们通过指定不同的区域来捕获不同的屏幕
	tempDir, err := ioutil.TempDir(consts.GetDefaultYakitBaseTempDir(), "multi-screen-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var screenFiles []string
	for i, display := range displays {
		screenFile := fmt.Sprintf("%s/screen_%d.png", tempDir, i)

		args := []string{
			"-f", "gdigrab",
			"-offset_x", strconv.Itoa(display.X),
			"-offset_y", strconv.Itoa(display.Y),
			"-video_size", fmt.Sprintf("%dx%d", display.Width, display.Height),
			"-i", "desktop",
			"-vframes", "1",
			"-y",
			screenFile,
		}

		cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
		if o.debug {
			cmd.Stderr = log.NewLogWriter(log.DebugLevel)
			log.Infof("capturing screen region %dx%d+%d+%d: %s", display.Width, display.Height, display.X, display.Y, cmd.String())
		}

		err := cmd.Run()
		if err != nil {
			log.Infof("failed to capture screen region: %v", err)
			continue
		}

		if utils.FileExists(screenFile) {
			screenFiles = append(screenFiles, screenFile)
		}
	}

	if len(screenFiles) == 0 {
		return nil, fmt.Errorf("failed to capture any screens")
	}

	return concatenateScreens(screenFiles, outputPath, o)
}

// captureMultipleScreensLinux 多屏幕截图并拼接 (Linux)
func captureMultipleScreensLinux(displays []DisplayInfo, outputPath string, o *options) (*FfmpegStreamResult, error) {
	if len(displays) <= 1 {
		return captureSingleScreenLinux(outputPath, o)
	}

	tempDir, err := ioutil.TempDir(consts.GetDefaultYakitBaseTempDir(), "multi-screen-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var screenFiles []string
	for i, display := range displays {
		screenFile := fmt.Sprintf("%s/screen_%d.png", tempDir, i)

		args := []string{
			"-f", "x11grab",
			"-video_size", fmt.Sprintf("%dx%d", display.Width, display.Height),
			"-i", fmt.Sprintf(":0.0+%d,%d", display.X, display.Y),
			"-vframes", "1",
			"-y",
			screenFile,
		}

		cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
		if o.debug {
			cmd.Stderr = log.NewLogWriter(log.DebugLevel)
			log.Infof("capturing screen region %dx%d+%d+%d: %s", display.Width, display.Height, display.X, display.Y, cmd.String())
		}

		err := cmd.Run()
		if err != nil {
			log.Infof("failed to capture screen region: %v", err)
			continue
		}

		if utils.FileExists(screenFile) {
			screenFiles = append(screenFiles, screenFile)
		}
	}

	if len(screenFiles) == 0 {
		return nil, fmt.Errorf("failed to capture any screens")
	}

	return concatenateScreens(screenFiles, outputPath, o)
}

// concatenateScreens 拼接多个屏幕截图
func concatenateScreens(screenFiles []string, outputPath string, o *options) (*FfmpegStreamResult, error) {
	if len(screenFiles) == 1 {
		// 只有一个文件，直接复制
		data, err := ioutil.ReadFile(screenFiles[0])
		if err != nil {
			return nil, fmt.Errorf("failed to read screen file: %w", err)
		}

		err = ioutil.WriteFile(outputPath, data, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write output file: %w", err)
		}

		return readScreenshotResult(outputPath)
	}

	// 构建 ffmpeg 拼接命令
	args := []string{}

	// 添加输入文件
	for _, file := range screenFiles {
		args = append(args, "-i", file)
	}

	// 构建 filter_complex 参数来水平拼接
	var filterStr strings.Builder
	for i := 0; i < len(screenFiles); i++ {
		if i > 0 {
			filterStr.WriteString("[tmp")
			filterStr.WriteString(strconv.Itoa(i - 1))
			filterStr.WriteString("]")
		}
		filterStr.WriteString("[")
		filterStr.WriteString(strconv.Itoa(i))
		filterStr.WriteString(":v]")

		if i == len(screenFiles)-1 {
			filterStr.WriteString("hstack=inputs=")
			filterStr.WriteString(strconv.Itoa(len(screenFiles)))
			filterStr.WriteString("[v]")
		} else if i > 0 {
			filterStr.WriteString("hstack=inputs=2[tmp")
			filterStr.WriteString(strconv.Itoa(i))
			filterStr.WriteString("];")
		}
	}

	// 对于两个屏幕的简化版本
	if len(screenFiles) == 2 {
		args = append(args, "-filter_complex", "[0:v][1:v]hstack=inputs=2[v]", "-map", "[v]")
	} else {
		// 多个屏幕使用完整的 filter_complex
		args = append(args, "-filter_complex", filterStr.String(), "-map", "[v]")
	}

	// 添加高质量参数
	args = append(args,
		"-pix_fmt", "rgba", // 使用 RGBA 像素格式保持质量
		"-compression_level", "0", // PNG 无损压缩
		"-pred", "1", // PNG 预测滤波器
		"-q:v", strconv.Itoa(o.frameQuality), // 使用配置的质量参数
		"-y", outputPath)

	cmd := exec.CommandContext(o.ctx, ffmpegBinaryPath, args...)
	if o.debug {
		cmd.Stderr = log.NewLogWriter(log.DebugLevel)
		log.Infof("concatenating %d screens: %s", len(screenFiles), cmd.String())
	}

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to concatenate screens: %w", err)
	}

	return readScreenshotResult(outputPath)
}
