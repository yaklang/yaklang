package aireactdeps

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// showRawStreamOutput 显示实时AI流（或在断点模式下缓冲）
func showRawStreamOutput(reader io.Reader, breakpointMode bool) {
	gs := GetGlobalState()

	gs.StreamingMutex.Lock()
	// 检查是否已经为此请求显示了流输出
	if gs.StreamDisplayed {
		gs.StreamingMutex.Unlock()
		// 只是消费流而不显示
		io.Copy(io.Discard, reader)
		return
	}

	// 先停止指示器
	stopActivitySpinner()

	// 在断点模式下，收集所有内容并在最后显示
	if breakpointMode {
		gs.StreamingActive = true
		gs.StreamDisplayed = true
		gs.StreamStartTime = time.Now()
		gs.StreamCharCount = 0
		gs.StreamingMutex.Unlock()

		// 收集所有内容而不显示
		var buffer []byte
		tempBuffer := make([]byte, 1024)
		for {
			n, err := reader.Read(tempBuffer)
			if n > 0 {
				buffer = append(buffer, tempBuffer[:n]...)
				gs.StreamingMutex.Lock()
				gs.StreamCharCount += n
				gs.StreamingMutex.Unlock()
			}
			if err != nil {
				break
			}
		}

		// 一次性显示完整内容
		elapsed := time.Since(gs.StreamStartTime)
		content := string(buffer)
		// 清理内容（删除控制字符）
		cleanContent := ""
		for _, r := range content {
			if r != '\n' && r != '\r' && r != '\t' && r != '\x00' {
				cleanContent += string(r)
			}
		}

		fmt.Printf("[stream]: %s\n", cleanContent)
		fmt.Printf("[stream]: [%d chars, %.1fs] done\n", gs.StreamCharCount, elapsed.Seconds())

		// 标记流已完成并在需要时触发断点
		markStreamCompleted()
		return
	}

	// 正常实时流模式
	if !gs.StreamingActive {
		gs.StreamingActive = true
		gs.StreamDisplayed = true
		gs.StreamStartTime = time.Now()
		gs.StreamCharCount = 0
		fmt.Print("[stream]: ")
	}
	gs.StreamingMutex.Unlock()

	const maxDisplayWidth = 60 // 最大显示宽度
	var displayBuffer []rune   // 缓冲区以存储显示内容
	var byteBuffer []byte      // 缓冲区以累积UTF-8解码的字节

	buffer := make([]byte, 1024) // 读取更大的块以获得更好的UTF-8处理
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			gs.StreamingMutex.Lock()
			gs.StreamCharCount += n

			// 追加到字节缓冲区
			byteBuffer = append(byteBuffer, buffer[:n]...)

			// 找到最后一个完整的UTF-8字符边界
			validEnd := len(byteBuffer)
			for validEnd > 0 {
				if utf8.ValidString(string(byteBuffer[:validEnd])) {
					break
				}
				validEnd--
			}

			if validEnd > 0 {
				// 将有效的UTF-8字节转换为字符串
				text := string(byteBuffer[:validEnd])

				// 过滤掉控制字符并添加到显示缓冲区
				for _, r := range text {
					if r != '\n' && r != '\r' && r != '\t' && r != '\x00' {
						displayBuffer = append(displayBuffer, r)
					}
				}

				// 保留下次迭代的剩余不完整字节
				byteBuffer = byteBuffer[validEnd:]

				// 实现滚动字幕效果
				if len(displayBuffer) > maxDisplayWidth {
					// 只保留最后的maxDisplayWidth个字符
					displayBuffer = displayBuffer[len(displayBuffer)-maxDisplayWidth:]
				}

				// 清除当前行并重绘
				fmt.Print("\r[stream]: ")
				fmt.Print(string(displayBuffer))

				// 添加填充以清除任何剩余字符
				padding := maxDisplayWidth - len(displayBuffer)
				if padding > 0 {
					fmt.Print(strings.Repeat(" ", padding))
				}
			}

			gs.StreamingMutex.Unlock()
		}
		if err != nil {
			break
		}
	}

	gs.StreamingMutex.Lock()
	gs.StreamingActive = false
	elapsed := time.Since(gs.StreamStartTime)
	// 在显示最终消息之前完全清除行
	fmt.Print("\r" + strings.Repeat(" ", maxDisplayWidth+20) + "\r")
	fmt.Printf("[stream]: [%d chars, %.1fs] done\n", gs.StreamCharCount, elapsed.Seconds())
	gs.StreamingMutex.Unlock()

	// 标记流已完成并在需要时触发断点
	markStreamCompleted()
}

// markStreamCompleted 标记流已完成并在需要时触发响应断点
func markStreamCompleted() {
	gs := GetGlobalState()
	gs.StreamingMutex.Lock()
	defer gs.StreamingMutex.Unlock()

	// 如果我们处于断点模式并且有待处理的响应，触发断点
	if gs.PendingResponse != nil {
		// 在调用断点之前重置状态以避免锁
		resp := gs.PendingResponse
		gs.PendingResponse = nil
		gs.StreamingMutex.Unlock()

		handleResponseBreakpoint(resp)

		gs.StreamingMutex.Lock()
	}
}

// showReasonStreamOutput 显示推理流
func showReasonStreamOutput(reader io.Reader, debugMode bool) {
	if debugMode {
		fmt.Print("\n[reasoning]: ")
		io.Copy(os.Stdout, reader)
		fmt.Print(" done\n")
	}
}

// stopActivitySpinner 停止活动指示器
func stopActivitySpinner() {
	gs := GetGlobalState()
	gs.SpinnerMutex.Lock()
	if gs.SpinnerActive {
		select {
		case gs.SpinnerStop <- true:
		default:
		}
	}
	gs.SpinnerMutex.Unlock()

	// 等待一下让指示器停止
	time.Sleep(50 * time.Millisecond)
	fmt.Print("\r                    \r") // 清除指示器行
}
