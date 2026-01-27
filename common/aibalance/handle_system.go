package aibalance

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// ==================== System Monitoring Handlers ====================

// handleMemoryStatsAPI returns current memory statistics for debugging
func (c *ServerConfig) handleMemoryStatsAPI(conn net.Conn, request *http.Request) {
	c.logInfo("Handling memory stats API request")

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"memory": map[string]interface{}{
			"alloc_mb":         memStats.Alloc / 1024 / 1024,
			"total_alloc_mb":   memStats.TotalAlloc / 1024 / 1024,
			"sys_mb":           memStats.Sys / 1024 / 1024,
			"heap_alloc_mb":    memStats.HeapAlloc / 1024 / 1024,
			"heap_sys_mb":      memStats.HeapSys / 1024 / 1024,
			"heap_idle_mb":     memStats.HeapIdle / 1024 / 1024,
			"heap_inuse_mb":    memStats.HeapInuse / 1024 / 1024,
			"heap_released_mb": memStats.HeapReleased / 1024 / 1024,
			"heap_objects":     memStats.HeapObjects,
			"num_gc":           memStats.NumGC,
			"goroutines":       runtime.NumGoroutine(),
		},
	})
}

// handleForceGCAPI forces garbage collection and memory release
func (c *ServerConfig) handleForceGCAPI(conn net.Conn, request *http.Request) {
	c.logInfo("Handling force GC API request")

	// Capture memory before GC
	var beforeStats runtime.MemStats
	runtime.ReadMemStats(&beforeStats)
	beforeAllocMB := beforeStats.Alloc / 1024 / 1024

	// Force GC and release memory to OS
	runtime.GC()
	debug.FreeOSMemory()
	runtime.GC()
	debug.FreeOSMemory()

	// Capture memory after GC
	var afterStats runtime.MemStats
	runtime.ReadMemStats(&afterStats)
	afterAllocMB := afterStats.Alloc / 1024 / 1024

	freedMB := int64(beforeAllocMB) - int64(afterAllocMB)
	if freedMB < 0 {
		freedMB = 0
	}

	c.logInfo("Force GC completed: before=%dMB, after=%dMB, freed=%dMB",
		beforeAllocMB, afterAllocMB, freedMB)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "Garbage collection completed",
		"before_mb": beforeAllocMB,
		"after_mb":  afterAllocMB,
		"freed_mb":  freedMB,
		"memory": map[string]interface{}{
			"alloc_mb":         afterStats.Alloc / 1024 / 1024,
			"heap_alloc_mb":    afterStats.HeapAlloc / 1024 / 1024,
			"heap_inuse_mb":    afterStats.HeapInuse / 1024 / 1024,
			"heap_released_mb": afterStats.HeapReleased / 1024 / 1024,
			"heap_objects":     afterStats.HeapObjects,
			"goroutines":       runtime.NumGoroutine(),
		},
	})
}

// handleGoroutineDumpAPI returns goroutine stack traces for debugging
func (c *ServerConfig) handleGoroutineDumpAPI(conn net.Conn, request *http.Request) {
	log.Warnf("[GOROUTINE_DUMP] API called, current goroutines: %d", runtime.NumGoroutine())

	// Get goroutine profile with debug=2 for full stacks
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 2)

	fullDump := buf.String()

	// Parse goroutines - split by "goroutine " prefix
	type goroutineSummary struct {
		Signature  string `json:"signature"`
		StackTrace string `json:"stack_trace"` // Top N lines of stack trace for debugging
		Count      int    `json:"count"`
		Sample     string `json:"sample_stack"` // Full sample stack for one goroutine
		FirstLine  string `json:"first_line"`   // The goroutine header line (e.g., "goroutine 123 [running]:")
	}

	goroutineMap := make(map[string]*goroutineSummary)

	// Split by double newline to get individual goroutine blocks
	blocks := strings.Split(fullDump, "\n\n")
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || !strings.HasPrefix(block, "goroutine ") {
			continue
		}

		// Extract stack trace information
		lines := strings.Split(block, "\n")
		var signature string
		var stackLines []string
		var firstLine string

		for i, line := range lines {
			if i == 0 {
				firstLine = line // Save "goroutine N [status]:" line
				continue
			}

			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "" {
				continue
			}

			// Collect stack trace lines (up to 6 lines = 3 function calls with file locations)
			if len(stackLines) < 6 {
				stackLines = append(stackLines, line)
			}

			// Extract signature from first function call (non-tab line with parentheses)
			if signature == "" && !strings.HasPrefix(line, "\t") && strings.Contains(trimmedLine, "(") {
				// This is a function call line like "runtime.gopark(0x0?, 0x0?, 0x0?, 0x0?, 0x0?)"
				idx := strings.Index(trimmedLine, "(")
				if idx > 0 {
					signature = trimmedLine[:idx]
				} else {
					signature = trimmedLine
				}
			}
		}

		if signature == "" {
			signature = "unknown"
		}

		// Build stack trace string
		stackTrace := strings.Join(stackLines, "\n")

		if info, exists := goroutineMap[signature]; exists {
			info.Count++
		} else {
			goroutineMap[signature] = &goroutineSummary{
				Signature:  signature,
				StackTrace: stackTrace,
				Count:      1,
				Sample:     block,
				FirstLine:  firstLine,
			}
		}
	}

	// Convert to sorted slice
	var summaries []goroutineSummary
	for _, info := range goroutineMap {
		summaries = append(summaries, *info)
	}

	// Sort by count (descending)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Count > summaries[j].Count
	})

	// Log top 5 for quick diagnosis
	for i, s := range summaries {
		if i >= 5 {
			break
		}
		log.Warnf("[GOROUTINE_DUMP] Top %d: %d goroutines in %s", i+1, s.Count, s.Signature)
	}

	topN := 15
	if len(summaries) < topN {
		topN = len(summaries)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":        true,
		"total":          runtime.NumGoroutine(),
		"unique_stacks":  len(summaries),
		"top_goroutines": summaries[:topN],
		"full_dump":      fullDump,
	})
}

// ==================== Static File Handler ====================

// serveStaticFile serves static files (CSS, JS) from the embedded filesystem or local filesystem
func (c *ServerConfig) serveStaticFile(conn net.Conn, path string) {
	// Extract filename from path (e.g., "/portal/static/portal.css" -> "portal.css")
	fileName := strings.TrimPrefix(path, "/portal/static/")
	if fileName == "" || strings.Contains(fileName, "..") {
		c.logError("Invalid static file request: %s", path)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\nInvalid path"))
		return
	}

	filePath := "templates/static/" + fileName

	// Determine Content-Type based on file extension
	var contentType string
	switch {
	case strings.HasSuffix(fileName, ".css"):
		contentType = "text/css; charset=utf-8"
	case strings.HasSuffix(fileName, ".js"):
		contentType = "application/javascript; charset=utf-8"
	case strings.HasSuffix(fileName, ".png"):
		contentType = "image/png"
	case strings.HasSuffix(fileName, ".jpg"), strings.HasSuffix(fileName, ".jpeg"):
		contentType = "image/jpeg"
	case strings.HasSuffix(fileName, ".svg"):
		contentType = "image/svg+xml"
	case strings.HasSuffix(fileName, ".ico"):
		contentType = "image/x-icon"
	default:
		contentType = "application/octet-stream"
	}

	var fileContent []byte
	var err error

	// Try to read from filesystem first (for development hot-reload)
	localPaths := []string{
		"common/aibalance/" + filePath,
		filePath,
		"../" + filePath,
	}

	for _, localPath := range localPaths {
		if _, statErr := os.Stat(localPath); statErr == nil {
			fileContent, err = os.ReadFile(localPath)
			if err == nil {
				break
			}
		}
	}

	// If not found in filesystem, try embedded FS
	if fileContent == nil {
		fileContent, err = templatesFS.ReadFile(filePath)
		if err != nil {
			c.logError("Failed to read static file from embedded FS '%s': %v", filePath, err)
			conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\nFile not found"))
			return
		}
	}

	// Build HTTP response with caching headers
	header := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: %s\r\n"+
		"Content-Length: %d\r\n"+
		"Cache-Control: public, max-age=3600\r\n"+
		"\r\n",
		contentType, len(fileContent))

	conn.Write([]byte(header))
	conn.Write(fileContent)
	c.logInfo("Served static file: %s (%d bytes)", fileName, len(fileContent))
}
