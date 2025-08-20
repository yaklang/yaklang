package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		modelPath    = flag.String("m", "", "模型路径")
		host         = flag.String("host", "127.0.0.1", "服务主机")
		port         = flag.String("port", "8080", "服务端口")
		ctxSize      = flag.String("ctx-size", "4096", "上下文大小")
		embedding    = flag.Bool("embedding", false, "嵌入模式")
		verbose      = flag.Bool("verbose-prompt", false, "详细提示")
		pooling      = flag.String("pooling", "last", "池化方式")
		contBatching = flag.Bool("cont-batching", false, "连续批处理")
		batchSize    = flag.String("batch-size", "1024", "批处理大小")
		threads      = flag.String("threads", "8", "线程数")
		help         = flag.Bool("help", false, "显示帮助")
	)

	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	// 输出启动信息
	fmt.Printf("Mock llama-server starting...\n")
	fmt.Printf("Model: %s\n", *modelPath)
	fmt.Printf("Host: %s\n", *host)
	fmt.Printf("Port: %s\n", *port)
	fmt.Printf("Context size: %s\n", *ctxSize)
	fmt.Printf("Embedding mode: %t\n", *embedding)
	fmt.Printf("Verbose prompt: %t\n", *verbose)
	fmt.Printf("Pooling: %s\n", *pooling)
	fmt.Printf("Continuous batching: %t\n", *contBatching)
	fmt.Printf("Batch size: %s\n", *batchSize)
	fmt.Printf("Threads: %s\n", *threads)

	// 验证端口
	portInt, err := strconv.Atoi(*port)
	if err != nil {
		log.Fatalf("Invalid port: %s", *port)
	}

	if portInt < 1 || portInt > 65535 {
		log.Fatalf("Port out of range: %d", portInt)
	}

	// 创建简单的 HTTP 服务器来模拟 llama-server
	mux := http.NewServeMux()

	// 模拟健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","model":"%s","embedding":%t}`, *modelPath, *embedding)
	})

	// 模拟嵌入端点
	mux.HandleFunc("/embedding", func(w http.ResponseWriter, r *http.Request) {
		if !*embedding {
			http.Error(w, "Embedding mode not enabled", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"embedding":[0.1,0.2,0.3,0.4,0.5]}`)
	})

	// 模拟根端点
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"message":"Mock llama-server","host":"%s","port":"%s","model":"%s"}`,
			*host, *port, *modelPath)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", *host, *port),
		Handler: mux,
	}

	// 启动服务器
	go func() {
		fmt.Printf("main: server is listening on http://%s:%s starting the main loop\n", *host, *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待一小段时间确保服务器启动
	time.Sleep(100 * time.Millisecond)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("Mock llama-server started successfully on %s:%s\n", *host, *port)
	fmt.Printf("Press Ctrl+C to stop...\n")

	// 等待信号
	<-sigChan
	fmt.Printf("\nShutting down mock llama-server...\n")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	fmt.Printf("Mock llama-server stopped\n")
}

// parseArgs 解析命令行参数（用于调试）
func parseArgs() {
	args := os.Args[1:]
	fmt.Printf("Command line arguments: %s\n", strings.Join(args, " "))
}

func init() {
	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 在启动时输出参数信息（用于调试）
	if len(os.Args) > 1 {
		parseArgs()
	}
}
