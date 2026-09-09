package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/utils/grpc_recovery"

	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	// 直接导入以触发 init 函数，替代原来的 depinjector
	// aiengine 现在通过 script_engine.go 中的直接导入来注册
	// yakgrpc 已经在下方导入，会自动注册 mcp.NewLocalClient

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/yak/cmd/yakcmds"

	systemLog "log"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	_ "github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/stdio"
	"github.com/yaklang/yaklang/common/schema"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/engineendpoint"
	"github.com/yaklang/yaklang/common/utils/grpc_auth"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/utils/umask"
	"github.com/yaklang/yaklang/common/yak"
	debugger "github.com/yaklang/yaklang/common/yak/interactive_debugger"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	// start pprof
	_ "net/http/pprof"
)

var (
	yakVersion string
	gitHash    string
	buildTime  string
	goVersion  string
)

const grpcReadyMarkerPrefix = "yak grpc ready "
const grpcPProfReadyMarkerPrefix = "yak grpc pprof ready "

type grpcReadyEvent struct {
	SchemaVersion int          `json:"schemaVersion"`
	Address       string       `json:"address"`
	Transport     string       `json:"transport"`
	InstanceId    string       `json:"instanceId"`
	EngineVersion string       `json:"engineVersion"`
	PhaseI18n     *schema.I18n `json:"phaseI18n"`
}

type grpcPProfReadyEvent struct {
	SchemaVersion int    `json:"schemaVersion"`
	Address       string `json:"address"`
}

func writeGRPCReadyEvent(writer io.Writer, address string, transport string, instanceId string) error {
	event := grpcReadyEvent{
		SchemaVersion: 2,
		Address:       address,
		Transport:     transport,
		InstanceId:    instanceId,
		EngineVersion: consts.GetYakVersion(),
		PhaseI18n:     grpcEventPhaseI18n("serve"),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "%s%s\n", grpcReadyMarkerPrefix, payload)
	return err
}

func writeGRPCPProfReadyEvent(writer io.Writer, address string) error {
	payload, err := json.Marshal(grpcPProfReadyEvent{
		SchemaVersion: 1,
		Address:       address,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "%s%s\n", grpcPProfReadyMarkerPrefix, payload)
	return err
}

func startGRPCPProfServer(writer io.Writer, listenAddress string) (*http.Server, string, error) {
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return nil, "", fmt.Errorf("listen for grpc pprof on %q: %w", listenAddress, err)
	}

	actualAddress := listener.Addr().String()
	if err := writeGRPCPProfReadyEvent(writer, actualAddress); err != nil {
		_ = listener.Close()
		return nil, "", fmt.Errorf("write grpc pprof ready event: %w", err)
	}

	server := &http.Server{
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("grpc pprof server failed: %v", err)
		}
	}()
	return server, actualAddress, nil
}

func initializeDatabase(projectDatabase string, profileDBName string, ssadb string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("initializeDatabase panic: %v\n%s", r, spew.Sdump(r))
		}
	}()
	if isVersionCommand() {
		return nil
	}
	// project and profile
	if err := consts.InitializeYakitDatabase(projectDatabase, profileDBName, ssadb); err != nil {
		return err
	}

	// cve
	_, err := consts.InitializeCVEDatabase()
	if err != nil {
		log.Warnf("initialized cve database failed: %v", err)
	}

	// 调用一些数据库初始化的操作
	err = yakit.CallPostInitDatabase()
	if err != nil {
		return utils.Errorf("CallPostInitDatabase failed: %s", err)
	}
	return nil
}

func isVersionCommand() bool {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "-version", "version":
			return true
		default:
			return false
		}
	}
	return false
}

// isHelpCommand 判断本次调用是否只需要输出帮助（help/--help/-h）。
// 这类调用不应触发数据库初始化：初始化会建库、导入内置插件并输出大量日志，
// 在 CI 中探测帮助文本时会拖慢命令甚至触发 SIGPIPE 误判。
func isHelpCommand() bool {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "help", "-h", "--help", "-help", "--help=true":
			return true
		}
	}
	// 子命令帮助: yak <command> --help / -h（第二参数直接是帮助标志）。
	// 只检查第二参数，避免把 -c/脚本内容里的 "--help" 误判为帮助调用。
	if len(os.Args) > 2 {
		switch os.Args[2] {
		case "--help", "-h", "-help", "--help=true":
			return true
		}
	}
	return false
}

func init() {
	// 取消掉 0022 的限制，让用户可以创建别人也能写的文件夹
	umask.Umask(0)
	systemLog.Default().SetOutput(io.Discard)

	// set double
	const GCPercentDefault = 8
	if strings.TrimSpace(os.Getenv("GOGC")) == "" {
		log.Debugf("GC Percent Origin: %v -> %v", debug.SetGCPercent(GCPercentDefault), GCPercentDefault)
	}
	/*
		进行一些必要初始化，永远不要再 init 中直接调用数据库，不然会破坏数据库加载的顺序
	*/
	log.Debugf(`Yaklang Engine %v Initializing`, yakVersion)

	log.Debugf("net default dns resolver prefer_go: %v strict_errors: %v", net.DefaultResolver.PreferGo, net.DefaultResolver.StrictErrors)
	if os.Getenv("GODEBUG") != "" {
		log.Infof("GODEBUG: %s", os.Getenv("GODEBUG"))
	}
	net.DefaultResolver.PreferGo = false
	net.DefaultResolver.StrictErrors = false
	switch runtime.GOOS {
	case "linux":
		// static compile issue for glibc (linux)
		net.DefaultResolver.PreferGo = true
		os.Setenv("GODEBUG", "netdns=go")
	}

	os.Setenv("YAKMODE", "vm")

	if yakVersion == "" {
		yakVersion = "dev"
	}
	consts.SetYakVersion(yakVersion)

	if gitHash == "" {
		gitHash = "-"
	}
	consts.SetYakGitHash(gitHash)

	if buildTime == "" {
		buildTime = time.Now().String()
	}
	consts.SetYakBuildTime(buildTime)

	if goVersion == "" {
		goVersion = runtime.Version()
	}

	/* 初始化数据库: 在 grpc 模式下，数据库应该不在 init 中使用 */
	ignoreInitDatabase := []string{"grpc", "check-secret-local-grpc", "fixup-database", "ai-http-gateway"}
	switch {
	case len(os.Args) > 1 && os.Args[1] == "mcp" && log.IsMCPStdioCommand(os.Args) && !stdio.IsWorker():
		// The stdio supervisor owns only the client transport. Its worker
		// initializes databases; this branch must not print the grpc banner.
	case len(os.Args) > 1 && slices.Contains(ignoreInitDatabase, os.Args[1]):
		log.Debug("grpc should not initialize database in func:init")
		fmt.Printf(`
┓ ┳┳━┓┳┏ ┳  ┳━┓┏┓┓┏━┓
┗┏┛┃━┫┣┻┓┃  ┃━┫┃┃┃┃ ┳
 ┇ ┛ ┇┇ ┛┇━┛┛ ┇┇┗┛┇━┛
    %v %v

`, consts.GetYakVersion(), "yaklang.io")

	case isVersionCommand(), isHelpCommand():
		// pass

	default:
		err := initializeDatabase("", "", "")
		if err != nil {
			log.Warnf("initialize database failed: %s", err)
		}
	}
	yaklib.SetEngineInterface(yak.NewScriptEngine(1000))
	// depinjector 已移除，相关 init 注册通过直接导入包来触发
	// - aiengine: 通过 yak/script_engine.go 导入 aiengine 包时注册
	// - yakgrpc: 在本文件已导入，会自动注册 mcp.NewLocalClient
	yak.InitYaklangLib()
}

var installSubCommand = cli.Command{
	Name:  "install",
	Usage: "Install Yak  (Add to ENV PATH)",
	Action: func(c *cli.Context) error {
		file, err := exec.LookPath(os.Args[0])
		if err != nil && !errors.Is(err, exec.ErrDot) {
			return utils.Errorf("fetch current binary yak path failed: %s", err)
		}

		absFile, err := filepath.Abs(file)
		if err != nil {
			return utils.Errorf("The absPath failed for[%v] reason: %v", file, err)
		}
		log.Infof("current yak binary: %v", absFile)

		originFp, err := os.Open(absFile)
		if err != nil {
			return utils.Errorf("open current yak binary failed: %s", err)
		}
		defer originFp.Close()

		var installed string
		switch runtime.GOOS {
		case "windows":
			systemRoot := os.Getenv("WINDIR")
			if systemRoot == "" {
				systemRoot = os.Getenv("windir")
			}
			if systemRoot == "" {
				systemRoot = os.Getenv("SystemRoot")
			}

			if systemRoot == "" {
				return utils.Errorf("cannot fetch windows system root dir")
			}

			installed = filepath.Join(systemRoot, "System32", "yak.exe")
		default:
			installed = "/usr/local/bin/yak"
		}

		if installed == "" {
			return utils.Errorf("load installed target failed. you can install yak manual")
		}
		if utils.GetFirstExistedFile(installed) != "" {
			err := os.RemoveAll(installed)
			if err != nil {
				return utils.Errorf("remove old yak binary failed: %s", err)
			}
		}

		fp, err := os.OpenFile(installed, os.O_CREATE|os.O_RDWR, os.ModePerm)
		if err != nil {
			return utils.Errorf("cannot write to %v ... check permission or ... dir existed?(安装失败，检查是否有 /usr/local/bin/ 的权限？或者尝试 sudo 执行本命令)", installed)
		}
		defer fp.Close()
		_, err = io.Copy(fp, originFp)
		if err != nil {
			os.RemoveAll(installed)
			return utils.Errorf("copy yak to %v failed: %s", installed, err)
		}
		log.Infof("installed yak... now you can exec `yak version` to check...")
		return nil
	},
}

var mirrorGRPCServerCommand = cli.Command{
	Name:  "xgrpc",
	Usage: "Start GRPC Server Local, and Auto-Create Tunnel for Remote Controll",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "server", Usage: "远程 Yak Bridge X 服务器"},
		cli.StringFlag{Name: "secret", Usage: "远程 Yak Bridge X 服务器密码"},
		cli.StringFlag{Name: "note", Usage: "可携带的基础信息"},
		cli.StringFlag{Name: "gen-tls-crt", Value: "build/"},
	},
	Hidden: true,
	Action: func(c *cli.Context) error {
		if c.String("note") == "" {
			return utils.Errorf("mirror grpc need basic info ... at least: 你必须设置 --note 参数，例如 --note zhangsan 以便服务器区分您")
		}

		secret := utils.RandStringBytes(30)
		port := utils.GetRandomAvailableTCPPort()
		go func() {
			for {
				err := c.App.Run([]string{
					"yak",
					"grpc",
					"--tls",
					"--secret", secret,
					"--host", "127.0.0.1",
					"--port", fmt.Sprint(port),
					"--gen-tls-crt", c.String("gen-tls-crt"),
				})
				if err != nil {
					log.Errorf("grpc panic: %s", err)
					continue
				}
			}
		}()
		err := utils.WaitConnect(utils.HostPort("127.0.0.1", port), 10)
		if err != nil {
			log.Errorf("run grpc failed: %s", err)
			return err
		}

		server := c.String("server")
		serverSecret := c.String("secret")

		pubpem, err := ioutil.ReadFile(filepath.Join(c.String("gen-tls-crt"), "yakit-grpc-cert.pem"))
		if err != nil {
			return err
		}
		for {
			err := cybertunnel.MirrorLocalPortToRemoteWithRegisterEx(
				true, pubpem, secret, c.String("note"),
				"tcp", "127.0.0.1", port,
				0, utils.RandStringBytes(10), server, serverSecret, context.Background(),
			)
			if err != nil {
				log.Errorf("cybertunnel.MirrorLocalPortToRemoteEx failed: %s", err)
				time.Sleep(time.Second)
				continue
			}
		}
	},
}

func slowLogUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	// 继续处理请求
	resp, err := handler(ctx, req)

	// 计算请求处理的时间
	elapsed := time.Since(start)
	log.Debugf("exec RPC: %s, took %v \n", info.FullMethod, elapsed)

	if elapsed > 250*time.Millisecond {
		logMsg := fmt.Sprintf("slow RPC: %s, took %v\n", info.FullMethod, elapsed)

		log.Warn(logMsg)
		// 打开文件，如果文件不存在则创建，如果文件存在则在文件末尾追加
		f, err := os.OpenFile("debug-slow.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			log.Println(err)
		}
		defer f.Close()

		// 将日志写入文件
		if _, err := f.WriteString(logMsg); err != nil {
			log.Println(err)
		}
	}

	return resp, err
}

var startGRPCServerCommand = cli.Command{
	Name:   "grpc",
	Usage:  "Start GRPC Server to Receive Connections",
	Hidden: false,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "home",
			Usage: "设置用户数据所在的位置，包含插件 / 数据库等",
		},
		cli.StringFlag{
			Name: "host", Value: "127.0.0.1",
			Usage: "启动 GRPC 服务器的本地地址",
		},
		cli.IntFlag{
			Name: "port", Value: 8087,
			// Name: "port", Value: 8080,
			Usage: "启动 GRPC 的端口",
		},
		cli.StringFlag{
			Name:  "frontend",
			Usage: "指定前端的名称，默认是空字符串，表示不限制",
		},
		cli.StringFlag{
			Name:  "secret",
			Usage: "启动 GRPC 的认证口令",
		},
		cli.BoolFlag{
			Name: "tls",
		},
		cli.StringFlag{
			Name:  "gen-tls-crt",
			Value: "build/",
		},
		cli.BoolFlag{
			Name:  "pprof",
			Usage: "手动 pprof 采集",
		},
		cli.StringFlag{
			Name:  "pprof-listen",
			Value: ":18080",
			Usage: "pprof HTTP 监听地址；性能自动化应使用 127.0.0.1:0",
		},
		cli.IntFlag{
			Name:  "pprof-block-rate",
			Value: 1,
			Usage: "runtime block profile 采样率；CPU-only 诊断可设为 0",
		},
		cli.Float64Flag{
			Name:  "auto-pprof",
			Usage: "指定 pprof 采集秒数间隔,eg. 10",
		},
		cli.BoolFlag{
			Name: "debug",
		},
		cli.StringFlag{
			Name:   "project-db",
			Usage:  "Specific Yakit Project DB Name, eg. yakit-default.db",
			EnvVar: "YAK_DEFAULT_PROJECT_DATABASE_NAME",
		},
		cli.StringFlag{
			Name:   "profile-db",
			Usage:  "Specific User-Data & Profile(Plugin) DB Name, eg yakit-profile-plugin.db",
			EnvVar: "YAK_DEFAULT_PROFILE_DATABASE_NAME",
		},

		cli.StringFlag{
			Name:   "ssa-db",
			Usage:  "Specific SSA Database  Name, eg default-yakssa.db",
			EnvVar: "SSA_DATABASE_RAW",
		},
		cli.BoolFlag{
			Name:  "disable-output",
			Usage: "禁止插件的一些输出",
		},
		cli.IntFlag{
			Name:  "reverse-port",
			Usage: "反连本地监听端口",
			Value: 0,
		},
		cli.BoolFlag{
			Name:  "disable-reverse-server",
			Usage: "关闭反连服务器（反连服务现在默认按需启动，保留该参数以兼容旧命令行）",
		},
		cli.StringFlag{
			Name:  "common-name",
			Usage: "设置证书的 Common Name, 默认为 Server",
			Value: "Server",
		},
		cli.StringFlag{
			Name:  "local-password",
			Usage: "本地密码认证，不使用 TLS；TCP 默认 127.0.0.1:9011（可用 --port 修改），unix/npipe 仅监听 --socket-path",
		},
		cli.StringFlag{
			Name:  "transport",
			Usage: "传输方式：tcp（默认）/ unix / npipe",
			Value: "tcp",
		},
		cli.StringFlag{
			Name:  "socket-path",
			Usage: "IPC socket 路径或管道名（仅 unix/npipe）；Unix 新建父目录默认 0700，保留已有目录权限，socket 为 0600",
		},
	},
	Action: func(c *cli.Context) (finalError error) {
		grpcStartTime := time.Now()
		var grpcPhase string = "init"
		var grpcReasonCode string
		var grpcReasonI18n *schema.I18n
		transport := c.String("transport")
		socketPath := c.String("socket-path")
		// Windows cache logging redirects os.Stdout asynchronously. Control events
		// must reach the parent even when a startup failure immediately exits.
		originalOutput := os.Stdout
		eventOutput := func() *os.File {
			if runtime.GOOS == "windows" {
				return originalOutput
			}
			return os.Stdout
		}
		defer func() {
			if finalError != nil {
				elapsedMs := time.Since(grpcStartTime).Milliseconds()
				reasonStr := finalError.Error()
				failedEvent := struct {
					SchemaVersion int          `json:"schemaVersion"`
					Phase         string       `json:"phase"`
					Reason        string       `json:"reason"`
					ReasonCode    string       `json:"reasonCode"`
					ElapsedMs     int64        `json:"elapsedMs"`
					Version       string       `json:"version"`
					PhaseI18n     *schema.I18n `json:"phaseI18n"`
					ReasonI18n    *schema.I18n `json:"reasonI18n"`
				}{
					SchemaVersion: 1,
					Phase:         grpcPhase,
					Reason:        reasonStr,
					ReasonCode:    grpcReasonCode,
					ElapsedMs:     elapsedMs,
					Version:       consts.GetYakVersion(),
					PhaseI18n:     grpcEventPhaseI18n(grpcPhase),
					ReasonI18n:    grpcReasonI18n,
				}
				payload, _ := json.Marshal(failedEvent)
				fmt.Fprintf(eventOutput(), "yak grpc failed %s\n", payload)
				log.Flush()
			}
		}()

		if err := validateEngineTransportOptions(c, true); err != nil {
			grpcReasonCode = "init_failed"
			grpcReasonI18n = grpcEventReasonI18n(grpcReasonCode)
			return err
		}
		if transport != "tcp" {
			if err := engineendpoint.PrepareListener(transport, socketPath); err != nil {
				grpcPhase = "listen"
				grpcReasonCode, _ = classifyEndpointListenError(transport, err)
				grpcReasonI18n = grpcEventReasonI18n(grpcReasonCode)
				return err
			}
		}
		// Reserve Windows pipes before database work, not just a racy existence
		// check. Every early return releases only this process's listener.
		reservedIPCListener, err := reserveWindowsIPCListener(transport, socketPath)
		if err != nil {
			grpcPhase = "listen"
			grpcReasonCode, _ = classifyEndpointListenError(transport, err)
			grpcReasonI18n = grpcEventReasonI18n(grpcReasonCode)
			return err
		}
		if reservedIPCListener != nil {
			defer reservedIPCListener.Close()
		}

		// 检查 local-password 模式
		localPassword := c.String("local-password")
		localRandomPasswordPort := 9011
		if localPassword != "" {
			// 检查互斥选项
			if c.IsSet("host") && c.String("host") != "127.0.0.1" {
				grpcPhase = "init"
				grpcReasonCode = "init_failed"
				grpcReasonI18n = grpcEventReasonI18n("init_failed")
				finalError = utils.Error("local-password is mutually exclusive with host option")
				return
			}
			if c.IsSet("port") {
				manualPort := c.Int("port")
				if manualPort <= 0 || manualPort > 65535 {
					grpcPhase = "init"
					grpcReasonCode = "init_failed"
					grpcReasonI18n = grpcEventReasonI18n("init_failed")
					finalError = utils.Error("local-password mode port must be between 1 and 65535")
					return
				}
				localRandomPasswordPort = manualPort
			}
			if c.IsSet("secret") {
				grpcPhase = "init"
				grpcReasonCode = "init_failed"
				grpcReasonI18n = grpcEventReasonI18n("init_failed")
				finalError = utils.Error("local-password is mutually exclusive with secret option")
				return
			}
			if c.IsSet("tls") && c.Bool("tls") {
				grpcPhase = "init"
				grpcReasonCode = "init_failed"
				grpcReasonI18n = grpcEventReasonI18n("init_failed")
				finalError = utils.Error("local-password is mutually exclusive with tls option")
				return
			}
			if c.IsSet("gen-tls-crt") && c.String("gen-tls-crt") != "build/" {
				grpcPhase = "init"
				grpcReasonCode = "init_failed"
				grpcReasonI18n = grpcEventReasonI18n("init_failed")
				finalError = utils.Error("local-password is mutually exclusive with gen-tls-crt option")
				return
			}

			log.Info("starting grpc server in local-password mode")
			if transport == "tcp" {
				log.Infof("local-password mode: port=%d, tls=false, password=***", localRandomPasswordPort)
			} else {
				log.Infof("local-password mode: transport=%s, endpoint=%s, tls=false, password=***", transport, socketPath)
			}
		}

		if c.String("home") != "" {
			os.Setenv("YAKIT_HOME", c.String("home"))
		}
		if c.Bool("pprof") && c.IsSet("auto-pprof") {
			grpcPhase = "init"
			grpcReasonCode = "init_failed"
			grpcReasonI18n = grpcEventReasonI18n("init_failed")
			finalError = utils.Error("Parameters 'pprof' and 'auto-pprof' cannot be set at the same time")
			return
		}
		if !c.Bool("pprof") && (c.IsSet("pprof-listen") || c.IsSet("pprof-block-rate")) {
			grpcPhase = "init"
			grpcReasonCode = "init_failed"
			grpcReasonI18n = grpcEventReasonI18n("init_failed")
			finalError = utils.Error("Parameters 'pprof-listen' and 'pprof-block-rate' require 'pprof'")
			return
		}
		if c.Int("pprof-block-rate") < 0 {
			grpcPhase = "init"
			grpcReasonCode = "init_failed"
			grpcReasonI18n = grpcEventReasonI18n("init_failed")
			finalError = utils.Error("Parameter 'pprof-block-rate' cannot be negative")
			return
		}
		if c.Bool("disable-output") {
			os.Setenv("YAK_DISABLE", "output")
		}
		consts.SetFrontendName(c.String("frontend"))

		cn := c.String("common-name")
		if cn == "" {
			cn = "Server"
		}

		enableProfile := c.Bool("pprof")
		if enableProfile {
			runtime.SetBlockProfileRate(c.Int("pprof-block-rate"))
			_, pprofAddress, err := startGRPCPProfServer(os.Stdout, c.String("pprof-listen"))
			if err != nil {
				grpcPhase = "pprof"
				grpcReasonCode = "pprof_failed"
				grpcReasonI18n = grpcEventReasonI18n("pprof_failed")
				finalError = err
				return
			}
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("---------------------------YAK GRPC PPROF-----------------------------")
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("USE: go tool pprof --seconds 30 http://" + pprofAddress + "/debug/pprof/profile")
		}
		pprofSec := c.Float64("auto-pprof")
		if pprofSec > 0 && c.IsSet("auto-pprof") {
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("---------------------------YAK GRPC AUTO PPROF-----------------------------")
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("USE: go tool pprof -http=:18080 pprof file")
			go startPProf(pprofSec)
		}
		log.Info("start to initialize database")

		err = initializeDatabase(c.String("project-db"), c.String("profile-db"), c.String("ssa-db"))
		if err != nil {
			log.Errorf("init database failed: %s", err)
			grpcPhase = "database"
			grpcReasonCode = "database_failed"
			grpcReasonI18n = grpcEventReasonI18n("database_failed")
			finalError = err
			return
		}

		/* 初始化数据库后进行权限修复 */
		base := consts.GetDefaultYakitBaseDir()
		projectDatabaseName := consts.GetDefaultYakitProjectDatabase(base)
		profileDatabaseName := consts.GetDefaultYakitPluginDatabase(base)
		log.Infof("use project db: %s", projectDatabaseName)
		log.Infof("use profile db: %s", profileDatabaseName)
		dialect, raw := consts.GetSSADataBaseInfo()
		log.Infof("use irify project db: [%s] %s", dialect, raw)

		yakit.TidyGeneralStorage(consts.GetGormProfileDatabase())

		certDir := c.String("gen-tls-crt")
		var caCertFile string = filepath.Join(certDir, "yakit-grpc-cert.pem")
		var caKeyFile string = filepath.Join(certDir, "yakit-grpc-key.pem")
		if transport == "tcp" && certDir != "" {
			err := os.MkdirAll(certDir, 0o777)
			if err != nil {
				log.Warnf("mkdir certdir[%s] failed: %s", certDir, err)
			}
		}

		// 确定使用的密码：local-password 优先级更高
		secret := c.String("secret")
		if localPassword != "" {
			secret = localPassword
		}

		streamInterceptors := []grpc.StreamServerInterceptor{grpc_recovery.StreamServerInterceptor()}
		unaryInterceptors := []grpc.UnaryServerInterceptor{grpc_recovery.UnaryServerInterceptor()}
		if secret != "" {
			auth := func(ctx context.Context) (context.Context, error) {
				userSecret, err := grpc_auth.AuthFromMD(ctx, "bearer")
				if err != nil {
					log.Errorf("secret schema[%v] missed", "bearer")
					return nil, err
				}
				if userSecret != secret {
					return nil, utils.Errorf("secret verify failed...")
				}
				return ctx, nil
			}
			streamInterceptors = append(streamInterceptors, grpc_auth.StreamServerInterceptor(auth))
			unaryInterceptors = append(unaryInterceptors, grpc_auth.UnaryServerInterceptor(auth))
		}
		debug := c.Bool("debug")
		if debug {
			unaryInterceptors = append(unaryInterceptors, slowLogUnaryInterceptor)
			log.SetLevel(log.DebugLevel)
		}
		log.Debug("start to create grpc schema...")
		grpcTrans := grpc.NewServer(
			grpc.ChainStreamInterceptor(streamInterceptors...),
			grpc.ChainUnaryInterceptor(unaryInterceptors...),
			grpc.MaxRecvMsgSize(100*1024*1024),
			grpc.MaxSendMsgSize(100*1024*1024),
		)
		reverse_port := c.Int("reverse-port")
		s, err := yakgrpc.NewServer(
			yakgrpc.WithReverseServerPort(reverse_port),
			yakgrpc.WithStartCacheLog(),
		)
		if err != nil {
			log.Errorf("build yakit server failed: %s", err)
			grpcPhase = "build_server"
			grpcReasonCode = "build_server_failed"
			grpcReasonI18n = grpcEventReasonI18n("build_server_failed")
			finalError = err
			return
		}
		ypb.RegisterYakServer(grpcTrans, s)

		// TCP 监听地址和端口；IPC 分支不使用这些值。
		var host string
		var port int
		if localPassword != "" {
			// local-password TCP 模式仅绑定 loopback，默认 9011，可显式指定端口。
			host = "127.0.0.1"
			port = localRandomPasswordPort
		} else {
			host = c.String("host")
			port = c.Int("port")
		}

		var lis net.Listener = reservedIPCListener

		// IPC endpoints are private; Unix may reclaim a verified stale socket.
		if transport == "unix" || transport == "npipe" {
			if socketPath == "" {
				grpcPhase = "init"
				grpcReasonCode = "init_failed"
				grpcReasonI18n = grpcEventReasonI18n("init_failed")
				finalError = utils.Errorf("socket-path is required when transport is %s", transport)
				return
			}
			log.Infof("start to listen (%s) on: %s", transport, socketPath)
			if lis == nil {
				lis, err = engineendpoint.Listen(transport, socketPath)
			}
			if err != nil {
				listenReason, hint := classifyEndpointListenError(transport, err)
				if listenReason == "" {
					listenReason = "listen_failed"
				}
				log.Errorf("failed to listen (%s): [%s] %s", transport, listenReason, hint)
				grpcPhase = "listen"
				grpcReasonCode = listenReason
				grpcReasonI18n = grpcEventReasonI18n(listenReason)
				finalError = utils.Wrapf(err, "[%s] %s", listenReason, hint)
				return
			}
		} else {
			// TCP 模式（默认）
			log.Infof("start to listen on: %v", utils.HostPort(host, port))

			// local-password 模式下强制不使用 TLS
			if localPassword == "" && c.Bool("tls") {
				// 签发证书
				var cert []byte
				var key []byte
				var err error

				cert, err = ioutil.ReadFile(caCertFile)
				if err != nil {
					log.Warnf("open ca-cert failed: %s", err)
				}
				key, err = ioutil.ReadFile(caKeyFile)
				if err != nil {
					log.Warnf("open ca-key failed: %s", err)
				}
				if cert == nil || key == nil {
					cert, key, err = tlsutils.GenerateSelfSignedCertKeyWithCommonNameEx(cn+" Root", cn+" Root", "", nil, nil, nil, false)
					if err != nil {
						grpcPhase = "cert"
						grpcReasonCode = "cert_failed"
						grpcReasonI18n = grpcEventReasonI18n("cert_failed")
						finalError = err
						return
					}
					err = ioutil.WriteFile(caCertFile, cert, 0o600)
					if err != nil {
						grpcPhase = "init"
						grpcReasonCode = "init_failed"
						grpcReasonI18n = grpcEventReasonI18n("init_failed")
						finalError = utils.Errorf("generate caCert[%s] failed: %s", caCertFile, err)
						return
					}
					err = ioutil.WriteFile(caKeyFile, key, 0o600)
					if err != nil {
						grpcPhase = "init"
						grpcReasonCode = "init_failed"
						grpcReasonI18n = grpcEventReasonI18n("init_failed")
						finalError = utils.Errorf("generate caKey[%s] failed: %s", caCertFile, err)
						return
					}
				}

				if cert != nil {
					log.Infof("Root CA (For Yakit)\n\n%v\n\n", string(cert))
				}

				serverCert, serverKey, err := tlsutils.SignServerCrtNKeyWithParams(cert, key, cn, time.Now().Add(100*365*24*time.Hour), false)
				if err != nil {
					grpcPhase = "cert"
					grpcReasonCode = "cert_failed"
					grpcReasonI18n = grpcEventReasonI18n("cert_failed")
					finalError = err
					return
				}
				serverCertIns, err := tlsutils.ParseCertificate(serverCert)
				if err == nil {
					text, err := tlsutils.CertificateText(serverCertIns)
					if err == nil {
						log.Infof("Server Certificate Fields \n\n%s\n\n", text)
					}
				}

				tlsConfig, err := tlsutils.GetX509ServerTlsConfig(cert, serverCert, serverKey)
				if err != nil {
					grpcPhase = "cert"
					grpcReasonCode = "cert_failed"
					grpcReasonI18n = grpcEventReasonI18n("cert_failed")
					finalError = err
					return
				}
				lis, err = tls.Listen("tcp", utils.HostPort(host, port), tlsConfig)
				if err != nil {
					listenReason, hint := classifyListenError(err)
					log.Errorf("failed to listen (tls): [%s] %s", listenReason, hint)
					grpcPhase = "listen"
					grpcReasonCode = listenReason
					grpcReasonI18n = grpcEventReasonI18n(listenReason)
					finalError = utils.Wrapf(err, "[%s] %s", listenReason, hint)
					return
				}
			} else {
				lis, err = net.Listen("tcp", utils.HostPort(host, port))
				if err != nil {
					listenReason, hint := classifyListenError(err)
					log.Errorf("failed to listen (tcp): [%s] %s", listenReason, hint)
					grpcPhase = "listen"
					grpcReasonCode = listenReason
					grpcReasonI18n = grpcEventReasonI18n(listenReason)
					finalError = utils.Wrapf(err, "[%s] %s", listenReason, hint)
					return
				}
			}
		}
		if reservedIPCListener == nil {
			defer lis.Close()
		}
		// IPC paths belong to this listener. A normal interrupt must release the
		// endpoint so a manual restart can reuse it. Unix also recovers stale
		// sockets on the next start if the process cannot clean up (e.g. SIGKILL).
		if transport != "tcp" {
			signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
			if transport == "unix" {
				signals = append(signals, syscall.SIGHUP)
			}
			signalCtx, stopSignals := signal.NotifyContext(context.Background(), signals...)
			defer stopSignals()
			done := make(chan struct{})
			defer close(done)
			go func() {
				select {
				case <-signalCtx.Done():
					grpcTrans.Stop()
				case <-done:
				}
			}()
		}
		s.StartAIReActScheduler()
		defer s.StopAIReActScheduler()

		actualAddress := lis.Addr().String()
		instanceId := utils.RandStringBytes(8)
		log.Infof("yak grpc listener ready on: %s", actualAddress)
		if err := writeGRPCReadyEvent(eventOutput(), actualAddress, transport, instanceId); err != nil {
			log.Warnf("write yak grpc ready event failed: %v", err)
		}

		log.Infof("start to startup grpc server(yak grpc ok)...")
		if transport != "tcp" {
			log.Infof("the current yak grpc listening on %s endpoint %s", transport, actualAddress)
		} else if host == "127.0.0.1" {
			if localPassword != "" {
				log.Infof("the current yak grpc running in local-password mode on '127.0.0.1:%d'", port)
			} else {
				log.Info("the current yak grpc for '127.0.0.1', if u want to connect from other host. use \n" +
					"    yak grpc --host 0.0.0.0")
			}
		}
		log.Info("yak grpc ok") // 勿删
		os.Stdout.WriteString("yak grpc ok\n")
		log.Flush()
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
			grpcPhase = "serve"
			grpcReasonCode = "serve_failed"
			grpcReasonI18n = grpcEventReasonI18n("serve_failed")
			finalError = err
			return
		}
		return nil
	},
}

// Unix keeps its existing preflight/stale-socket lifecycle. Windows can reserve
// the kernel pipe before opening databases without creating filesystem entries.
func reserveWindowsIPCListener(transport, endpoint string) (net.Listener, error) {
	if runtime.GOOS != "windows" || transport != "npipe" {
		return nil, nil
	}
	return engineendpoint.Listen(transport, endpoint)
}

func newEngineSecret() (string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(secret[:]), nil
}

func legacyCheckReason(code string) string {
	switch code {
	case databaseError:
		return "database error"
	case dialGrpcServerFailed:
		return "dial grpc server failed"
	case callVersionFailed:
		return "call Version RPC failed"
	case buildYakGrpcServer:
		return "build yak grpc server failed"
	case waitConnectFailed:
		return "waiting grpc listener failed"
	case tcpBindDenied, tcpBindInUse, tcpBindGeneric:
		return "net.Listen(tcp, addr) failed"
	default:
		return code
	}
}

func validateEngineTransportOptions(ctx *cli.Context, requirePassword bool) error {
	transport, endpoint := ctx.String("transport"), ctx.String("socket-path")
	if err := engineendpoint.Validate(transport, endpoint); err != nil {
		return err
	}
	if transport == "tcp" {
		return nil
	}
	if ctx.Bool("tls") || ctx.IsSet("gen-tls-crt") {
		return utils.Error("IPC transport does not support TLS options")
	}
	if ctx.IsSet("host") || ctx.IsSet("port") {
		return utils.Error("IPC transport uses socket-path; host and port options are not supported")
	}
	secret := ctx.String("local-password")
	if secret == "" {
		secret = ctx.String("secret")
	}
	if requirePassword && strings.Trim(strings.TrimSpace(secret), "*") == "" {
		return utils.Error("IPC transport requires a non-empty secret or local-password")
	}
	return nil
}

const (
	databaseError        = "database_error"
	dialGrpcServerFailed = "dial_failed"
	callVersionFailed    = "version_rpc_failed"
	buildYakGrpcServer   = "build_server_failed"
	waitConnectFailed    = "wait_connect_failed"

	// listen error sub-categories
	tcpBindDenied      = "tcp_bind_denied"
	tcpBindInUse       = "tcp_bind_in_use"
	tcpBindGeneric     = "tcp_bind_failed"
	ipcEndpointInvalid = "ipc_endpoint_invalid"
	ipcBindDenied      = "ipc_bind_denied"
	ipcBindInUse       = "ipc_bind_in_use"
	ipcBindGeneric     = "ipc_bind_failed"
)

func classifyEndpointListenError(transport string, err error) (reason string, userHint string) {
	if transport == "tcp" {
		return classifyListenError(err)
	}
	if errors.Is(err, engineendpoint.ErrInvalidDirectory) {
		return ipcEndpointInvalid, err.Error()
	}
	// Win32 named pipes return ERROR_ACCESS_DENIED (5), not Winsock's
	// WSAEACCES (10013). os.ErrPermission recognizes both native platforms.
	if errors.Is(err, os.ErrPermission) {
		return ipcBindDenied, err.Error()
	}
	reason, _ = classifyListenError(err)
	switch reason {
	case tcpBindDenied:
		reason = ipcBindDenied
	case tcpBindInUse:
		reason = ipcBindInUse
	default:
		reason = ipcBindGeneric
	}
	if err != nil {
		userHint = err.Error()
	}
	return
}

// classifyListenError inspects a net.Listen / tls.Listen error and returns
// a structured reason string plus a human-readable hint for diagnostics.
// This avoids lumping permission denials, address-in-use and other failures
// under a single "port occupied" message.
func classifyListenError(err error) (reason string, userHint string) {
	if err == nil {
		return tcpBindGeneric, ""
	}
	if errors.Is(err, syscall.EADDRINUSE) || (runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10048))) {
		return tcpBindInUse, "Address already in use: choose another endpoint"
	}
	if errors.Is(err, syscall.EACCES) || (runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10013))) {
		return tcpBindDenied, "Permission denied: check the endpoint and system policy"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if se, ok := opErr.Err.(*os.SyscallError); ok {
			errno, ok := se.Err.(syscall.Errno)
			if !ok {
				return tcpBindGeneric, err.Error()
			}
			// WSAEACCES (10013) on Windows, EACCES on Unix
			if errno == syscall.EACCES || (runtime.GOOS == "windows" && errno == 10013) {
				return tcpBindDenied, "Permission denied: the port may require elevated privileges or is blocked by system policy"
			}
			// WSAEADDRINUSE (10048) on Windows, EADDRINUSE on Unix
			if errno == syscall.EADDRINUSE || (runtime.GOOS == "windows" && errno == 10048) {
				return tcpBindInUse, "Address already in use: another process is listening on this port"
			}
		}
	}
	return tcpBindGeneric, err.Error()
}

func runCheckSecretCleanupWithTimeout(name string, timeout time.Duration, cleanup func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		cleanup()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		log.Warnf("%s timed out after %s, continue check-secret-local-grpc shutdown", name, timeout)
	}
}

var checkSecretLocalGRPCServerCommand = cli.Command{
	Name:  "check-secret-local-grpc",
	Usage: "Check if local GRPC server with secret can be started and accessed",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "client-password",
			Usage: "客户端模式：使用指定密码连接到现有服务器进行测试，不启动服务器",
		},
		cli.IntFlag{
			Name:  "p,port",
			Usage: "Specific port to start local grpc server (default 9011)",
			Value: 9011,
		},
		cli.StringFlag{
			Name:   "project-db",
			Usage:  "Specific Yakit Project DB Name, eg. yakit-default.db",
			EnvVar: "YAK_DEFAULT_PROJECT_DATABASE_NAME",
		},
		cli.StringFlag{
			Name:   "profile-db",
			Usage:  "Specific User-Data & Profile(Plugin) DB Name, eg yakit-profile-plugin.db",
			EnvVar: "YAK_DEFAULT_PROFILE_DATABASE_NAME",
		},

		cli.StringFlag{
			Name:   "ssa-db",
			Usage:  "Specific SSA Database  Name, eg default-yakssa.db",
			EnvVar: "SSA_DATABASE_RAW",
		},
		cli.StringFlag{
			Name:  "transport",
			Usage: "传输方式：tcp（默认）/ unix / npipe",
			Value: "tcp",
		},
		cli.StringFlag{
			Name:  "socket-path",
			Usage: "IPC socket 路径或管道名（仅 unix/npipe）；Unix 新建父目录默认 0700，保留已有目录权限，socket 为 0600",
		},
	},
	Action: func(ctx *cli.Context) (finalError error) {
		var port = ctx.Int("port")

		addr := utils.HostPort("127.0.0.1", port)

		clientPassword := ctx.String("client-password")
		transport := ctx.String("transport")
		socketPath := ctx.String("socket-path")
		if transport != "tcp" {
			addr = socketPath
		}

		projectPath := ctx.String("project-db")
		profilePath := ctx.String("profile-db")
		ssaPath := ctx.String("ssa-db")

		var version = consts.GetYakVersion()
		secret, secretErr := newEngineSecret()

		var reason string
		var phase string = "init"
		var checkSecretReasonI18n *schema.I18n
		checkSecretStartTime := time.Now()

		defer func() {
			const extractorFlag = `50551aa97b5aa5ae8a3c3243ac60a8a7`

			if err := recover(); err != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
				finalError = utils.Errorf("panic: %v\n%s", err, spew.Sdump(string(debug.Stack())))
			}

			// panic fallback: if we crashed without setting reason/phase/i18n,
			// provide a generic error so front-end always gets usable fields.
			if !utils.IsNil(finalError) && reason == "" {
				reason = "unexpected_error"
				phase = "init"
				checkSecretReasonI18n = grpcEventReasonI18n("unexpected_error")
			}

			m := omap.NewGeneralOrderedMap()
			ok := utils.IsNil(finalError)
			var info string
			if ok {
				info = ""
			} else {
				info = utils.InterfaceToString(finalError)
			}
			m.Set("schemaVersion", 1)
			m.Set("ok", ok)
			m.Set("reason", []string{legacyCheckReason(reason)})
			m.Set("phase", phase)
			m.Set("elapsedMs", time.Since(checkSecretStartTime).Milliseconds())
			m.Set("info", info)
			m.Set("transport", transport)
			if transport == "tcp" {
				m.Set("host", "127.0.0.1")
				m.Set("port", port)
				m.Set("addr", utils.HostPort("127.0.0.1", port))
			} else {
				m.Set("host", "")
				m.Set("port", 0)
				m.Set("addr", socketPath)
			}
			// This is a machine protocol credential consumed by old Yakit, not a log field.
			m.Set("secret", secret)
			m.Set("version", version)
			m.Set("reasonCode", reason)
			// i18n: bilingual labels set directly at each failure point.
			phaseLabel := grpcEventPhaseI18n(phase)
			if phaseLabel != nil {
				m.Set("phaseI18n", phaseLabel)
			}
			if checkSecretReasonI18n != nil {
				m.Set("reasonI18n", checkSecretReasonI18n)
			}
			result := string(m.Jsonify())
			fmt.Printf("\n<json-%v>\n%v\n</json-%v>\n\n", extractorFlag, result, extractorFlag)
		}()

		if secretErr != nil {
			reason = "unexpected_error"
			return secretErr
		}
		if err := validateEngineTransportOptions(ctx, false); err != nil {
			reason = "init_failed"
			checkSecretReasonI18n = grpcEventReasonI18n(reason)
			return err
		}

		// 客户端模式：只连接服务器测试
		if clientPassword != "" {
			log.Info("running in client mode, connecting to existing server...")
			if transport == "tcp" {
				log.Infof("target: %s", addr)
			} else {
				log.Infof("target: %s (%s)", socketPath, transport)
			}

			// 创建带超时的 context（10 秒）
			dialCtx, dialCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer dialCancel()

			// 创建客户端连接
			log.Infof("connecting to grpc server with password (timeout: 10s)...")
			var conn *grpc.ClientConn
			var err error
			if transport == "tcp" {
				conn, err = grpc.DialContext(
					dialCtx,
					addr,
					grpc.WithInsecure(),
					grpc.WithBlock(),
					grpc.WithDefaultCallOptions(
						grpc.MaxCallRecvMsgSize(100*1024*1024),
						grpc.MaxCallSendMsgSize(100*1024*1024),
					),
				)
			} else {
				conn, err = grpc.DialContext(
					dialCtx,
					socketPath,
					grpc.WithInsecure(),
					grpc.WithBlock(),
					grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
						return engineendpoint.DialContext(ctx, transport, socketPath)
					}),
					grpc.WithDefaultCallOptions(
						grpc.MaxCallRecvMsgSize(100*1024*1024),
						grpc.MaxCallSendMsgSize(100*1024*1024),
					),
				)
			}
			if err != nil {
				log.Errorf("failed to dial grpc server: %s", err)
				fmt.Printf("\n[FAILED] Cannot connect to server at %s\n", addr)
				fmt.Printf("Error: %s\n", err)
				fmt.Printf("Please check if:\n")
				// Distinguish gRPC error codes for better diagnostics
				if transport != "tcp" {
					fmt.Printf("  1. The engine is running on this %s endpoint\n", transport)
					fmt.Printf("  2. The socket path or pipe name matches the engine's ready event\n")
					fmt.Printf("  3. The current user has permission to connect to the IPC endpoint\n")
				} else if strings.Contains(err.Error(), "Unavailable") || strings.Contains(err.Error(), "unavailable") {
					fmt.Printf("  1. The yak engine is not running on port %d\n", port)
					fmt.Printf("  2. The connection was refused by the target\n")
				} else if strings.Contains(err.Error(), "DeadlineExceeded") || strings.Contains(err.Error(), "context deadline exceeded") {
					fmt.Printf("  1. Connection timeout (10s exceeded)\n")
					fmt.Printf("  2. The server is too slow to respond or network is unreachable\n")
				} else {
					fmt.Printf("  1. Server is running on port %d\n", port)
					fmt.Printf("  2. Local firewall is not blocking the connection\n")
					fmt.Printf("  3. Connection timeout (10s exceeded)\n")
				}
				fmt.Printf("\n")
				finalError = utils.Wrap(err, dialGrpcServerFailed)
				phase = "dial"
				reason = dialGrpcServerFailed
				checkSecretReasonI18n = grpcEventReasonI18n(dialGrpcServerFailed)
				return
			}
			defer conn.Close()

			client := ypb.NewYakClient(conn)

			// 创建带认证和超时的 context（10 秒）
			rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer rpcCancel()
			rpcCtx = metadata.AppendToOutgoingContext(
				rpcCtx,
				"authorization", fmt.Sprintf("bearer %v", clientPassword),
			)

			// 调用 Version 接口
			log.Infof("calling Version RPC with authentication (timeout: 10s)...")
			versionResp, err := client.Version(rpcCtx, &ypb.Empty{})
			if err != nil {
				log.Errorf("failed to call Version: %s", err)
				fmt.Printf("\n[FAILED] Authentication or RPC call failed\n")
				fmt.Printf("Error: %s\n", err)
				fmt.Printf("Please check if:\n")
				fmt.Printf("  1. The password is correct\n")
				fmt.Printf("  2. The server is running in local-password mode\n\n")
				finalError = utils.Wrap(err, callVersionFailed)
				phase = "version_rpc"
				reason = callVersionFailed
				checkSecretReasonI18n = grpcEventReasonI18n(callVersionFailed)
				return
			}

			log.Infof("Version RPC successful: %s", versionResp.Version)
			fmt.Printf("\n[SUCCESS] Client connection test passed\n")
			fmt.Printf("  Server: %s\n", addr)
			fmt.Printf("  Password: ***\n")
			fmt.Printf("  Version: %s\n\n", versionResp.Version)
			version = versionResp.Version
			finalError = nil
			return
		}

		if transport != "tcp" {
			if err := engineendpoint.PrepareListener(transport, socketPath); err != nil {
				phase = "listen"
				reason, _ = classifyEndpointListenError(transport, err)
				checkSecretReasonI18n = grpcEventReasonI18n(reason)
				return err
			}
		}

		// Fail on an occupied Windows pipe before opening or migrating databases.
		reservedIPCListener, err := reserveWindowsIPCListener(transport, socketPath)
		if err != nil {
			phase = "listen"
			reason, _ = classifyEndpointListenError(transport, err)
			checkSecretReasonI18n = grpcEventReasonI18n(reason)
			return err
		}
		if reservedIPCListener != nil {
			defer reservedIPCListener.Close()
		}

		if err := consts.InitializeYakitDatabase(projectPath, profilePath, ssaPath); err != nil {
			finalError = err
			log.Errorf("failed to open database: %s", err)
			fmt.Printf("\nPlease run `%s fixup-database` ", os.Args[0])
			phase = "database"
			reason = databaseError
			checkSecretReasonI18n = grpcEventReasonI18n(databaseError)
			return
		}

		// 服务器模式：启动测试服务器
		log.Info("running in server mode, starting test server...")

		// 监听
		var lis net.Listener = reservedIPCListener
		if transport == "tcp" {
			lis, err = net.Listen("tcp", addr)
		} else if lis == nil {
			lis, err = engineendpoint.Listen(transport, socketPath)
		}
		if err != nil {
			listenReason, hint := classifyEndpointListenError(transport, err)
			if listenReason == "" {
				listenReason = "listen_failed"
			}
			if transport == "tcp" {
				log.Errorf("failed to listen on port %d: [%s] %s", port, listenReason, hint)
			} else {
				log.Errorf("failed to listen (%s) on %s: [%s] %s", transport, socketPath, listenReason, hint)
			}
			if transport == "tcp" {
				fmt.Printf("\n[FAILED] Cannot listen on port %d\n", port)
			} else {
				fmt.Printf("\n[FAILED] Cannot listen on %s endpoint %s\n", transport, socketPath)
			}
			fmt.Printf("Reason: %s\n", listenReason)
			fmt.Printf("Hint: %s\n", hint)
			fmt.Printf("Please check if:\n")
			switch listenReason {
			case tcpBindDenied:
				fmt.Printf("  1. The port requires elevated privileges\n")
				fmt.Printf("  2. System policy or firewall is blocking the port\n")
			case tcpBindInUse:
				fmt.Printf("  1. Another yak instance is already running on this port\n")
				fmt.Printf("  2. Another application is using this port\n")
			default:
				fmt.Printf("  1. %s\n", hint)
			}
			fmt.Printf("\n")
			finalError = utils.Wrapf(err, "[%s] %s", listenReason, hint)
			phase = "listen"
			reason = listenReason
			checkSecretReasonI18n = grpcEventReasonI18n(listenReason)
			return
		}
		if reservedIPCListener == nil {
			defer lis.Close()
		}

		log.Info("generated random secret for testing: ***")

		// 创建 GRPC 服务器
		auth := func(authCtx context.Context) (context.Context, error) {
			userSecret, err := grpc_auth.AuthFromMD(authCtx, "bearer")
			if err != nil {
				log.Errorf("secret schema[%v] missed", "bearer")
				return nil, err
			}
			if userSecret != secret {
				return nil, utils.Errorf("secret verify failed...")
			}
			return authCtx, nil
		}

		streamInterceptors := []grpc.StreamServerInterceptor{
			grpc_recovery.StreamServerInterceptor(),
			grpc_auth.StreamServerInterceptor(auth),
		}
		unaryInterceptors := []grpc.UnaryServerInterceptor{
			grpc_recovery.UnaryServerInterceptor(),
			grpc_auth.UnaryServerInterceptor(auth),
		}

		grpcTrans := grpc.NewServer(
			grpc.ChainStreamInterceptor(streamInterceptors...),
			grpc.ChainUnaryInterceptor(unaryInterceptors...),
			grpc.MaxRecvMsgSize(100*1024*1024),
			grpc.MaxSendMsgSize(100*1024*1024),
		)
		// 初始化服务器
		s, err := yakgrpc.NewServer(
			yakgrpc.WithInitFacadeServer(false),
		)
		if err != nil {
			log.Errorf("build yakit server failed: %s", err)
			finalError = utils.Wrap(err, buildYakGrpcServer)
			phase = "build_server"
			reason = buildYakGrpcServer
			checkSecretReasonI18n = grpcEventReasonI18n(buildYakGrpcServer)
			return
		}
		ypb.RegisterYakServer(grpcTrans, s)

		// 启动服务器
		log.Infof("starting test grpc server on %s", addr)
		go func() {
			err := grpcTrans.Serve(lis)
			if err != nil {
				log.Errorf("grpc serve error: %s", err)
			}
		}()
		defer runCheckSecretCleanupWithTimeout("stop test grpc server", time.Second, grpcTrans.Stop)

		// 等待服务器启动
		if transport == "tcp" {
			if err := utils.WaitConnect(addr, 5); err != nil {
				log.Errorf("failed to connect to server, start local port listener failed: %s", err)
				finalError = utils.Wrap(err, "waiting grpc listener failed")
				phase = "wait_connect"
				reason = waitConnectFailed
				checkSecretReasonI18n = grpcEventReasonI18n(waitConnectFailed)
				return
			}
		} else {
			// Listen has already created the endpoint. One context-bounded dial is enough;
			// the subsequent authenticated Version RPC verifies server readiness.
			waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
			connection, waitErr := engineendpoint.DialContext(waitCtx, transport, socketPath)
			waitCancel()
			if waitErr != nil {
				finalError = utils.Wrap(waitErr, "waiting ipc listener failed")
				phase = "wait_connect"
				reason = waitConnectFailed
				checkSecretReasonI18n = grpcEventReasonI18n(waitConnectFailed)
				return
			}
			connection.Close()
		}

		// 创建带超时的 context（10 秒）
		dialCtx, dialCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dialCancel()

		// 创建客户端连接
		log.Infof("connecting to test grpc server (timeout: 10s)...")
		var conn *grpc.ClientConn
		if transport == "tcp" {
			conn, err = grpc.DialContext(
				dialCtx,
				addr,
				grpc.WithInsecure(),
				grpc.WithBlock(),
				grpc.WithDefaultCallOptions(
					grpc.MaxCallRecvMsgSize(100*1024*1024),
					grpc.MaxCallSendMsgSize(100*1024*1024),
				),
			)
		} else {
			conn, err = grpc.DialContext(
				dialCtx,
				socketPath,
				grpc.WithInsecure(),
				grpc.WithBlock(),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return engineendpoint.DialContext(ctx, transport, socketPath)
				}),
				grpc.WithDefaultCallOptions(
					grpc.MaxCallRecvMsgSize(100*1024*1024),
					grpc.MaxCallSendMsgSize(100*1024*1024),
				),
			)
		}
		if err != nil {
			log.Errorf("failed to dial grpc server: %s", err)
			finalError = utils.Wrap(err, dialGrpcServerFailed)
			phase = "dial"
			reason = dialGrpcServerFailed
			checkSecretReasonI18n = grpcEventReasonI18n(dialGrpcServerFailed)
			return
		}
		defer runCheckSecretCleanupWithTimeout("close test grpc client", time.Second, func() {
			_ = conn.Close()
		})

		client := ypb.NewYakClient(conn)

		// 创建带认证和超时的 context（10 秒）
		rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rpcCancel()
		rpcCtx = metadata.AppendToOutgoingContext(
			rpcCtx,
			"authorization", fmt.Sprintf("bearer %v", secret),
		)

		// 调用 Version 接口
		log.Infof("calling Version RPC (timeout: 10s)...")
		versionResp, err := client.Version(rpcCtx, &ypb.Empty{})
		if err != nil {
			log.Errorf("failed to call Version: %s", err)
			finalError = utils.Wrap(err, callVersionFailed)
			phase = "version_rpc"
			reason = callVersionFailed
			checkSecretReasonI18n = grpcEventReasonI18n(callVersionFailed)
			return
		}

		log.Infof("Version RPC successful: %s", versionResp.Version)
		fmt.Printf("\n[SUCCESS] Local GRPC server with secret authentication test passed\n")
		if transport == "tcp" {
			fmt.Printf("  Port: %d\n", port)
		} else {
			fmt.Printf("  Endpoint: %s (%s)\n", socketPath, transport)
		}
		fmt.Printf("  Secret: ***\n")
		fmt.Printf("  Version: %s\n\n", versionResp.Version)
		version = versionResp.Version
		finalError = nil

		return
	},
}

var fixupDatabaseCommand = cli.Command{
	Name:  "fixup-database",
	Usage: "Fix Yaklang Database Issues",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:   "project-db",
			Usage:  "Specific Yakit Project DB Name, eg. yakit-default.db",
			EnvVar: "YAK_DEFAULT_PROJECT_DATABASE_NAME",
		},
		cli.StringFlag{
			Name:   "profile-db",
			Usage:  "Specific User-Data & Profile(Plugin) DB Name, eg yakit-profile-plugin.db",
			EnvVar: "YAK_DEFAULT_PROFILE_DATABASE_NAME",
		},

		cli.StringFlag{
			Name:   "ssa-db",
			Usage:  "Specific SSA Database  Name, eg default-yakssa.db",
			EnvVar: "SSA_DATABASE_RAW",
		},
	},
	Action: func(ctx *cli.Context) (finalError error) {
		projectPath := ctx.String("project-db")
		profilePath := ctx.String("profile-db")
		ssaPath := ctx.String("ssa-db")

		consts.SetDefaultYakitProfileDatabaseName(profilePath)
		consts.SetDefaultYakitProjectDatabaseName(projectPath)
		consts.SetSSADatabaseInfo(ssaPath)

		needUserDeletePath := []string{}
		defer func() {
			if err := recover(); err != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
				finalError = utils.Errorf("panic: %v\n%s", err, spew.Sdump(string(debug.Stack())))
			}
			const extractorFlag = `a30b05e69086b63acff5dec94727ab5c`

			m := omap.NewGeneralOrderedMap()
			ok := utils.IsNil(finalError)
			var info string
			if ok {
				info = ""
			} else {
				info = utils.InterfaceToString(finalError)
			}
			m.Set("ok", ok)
			// failed recreate path
			m.Set("path", needUserDeletePath) // need show in front for user  handler this file
			m.Set("info", info)
			result := string(m.Jsonify())
			fmt.Printf("\n<json-%v>\n%v\n</json-%v>\n\n", extractorFlag, result, extractorFlag)
		}()

		checkAndRemove := func(
			path string,
			open func(string) error,
		) error {
			if err := open(path); err == nil {
				log.Infof("open database success: %s", path)
				return nil
			}

			if !utils.IsFile(path) {
				return utils.Errorf("please check connect: %s", path)
			}

			log.Infof("remove file: %s", path)
			if err := os.Remove(path); err != nil {
				needUserDeletePath = append(needUserDeletePath, path)
				return utils.Errorf("Remove path [%s] error: %s", path, err)
			}
			if err := open(path); err != nil {
				// should not reach here
				log.Infof("recreate database ok: %s", path)
				needUserDeletePath = append(needUserDeletePath, path)
				return utils.Errorf("recreate database failed: %s", path)
			}
			return nil
		}
		log.Debug("start to loading gorm project/profile database")

		baseDir := consts.GetDefaultYakitBaseDir()
		projectPath = consts.GetDefaultYakitProjectDatabase(baseDir)
		profilePath = consts.GetDefaultYakitPluginDatabase(baseDir)
		_, ssaPath = consts.GetSSADataBaseInfo()

		if err := checkAndRemove(profilePath, func(s string) error {
			_, err := consts.CreateProfileDatabase(s)
			return err
		}); err != nil {
			return err
		}

		if err := checkAndRemove(projectPath, func(s string) error {
			_, err := consts.CreateProjectDatabase(s)
			return err
		}); err != nil {
			return err
		}

		if err := checkAndRemove(ssaPath, func(s string) error {
			ssaDatabaseDialect, ssaDatabaseRaw := consts.GetSSADataBaseInfo()
			_, err := consts.CreateSSAProjectDatabase(ssaDatabaseDialect, ssaDatabaseRaw)
			return err
		}); err != nil {
			return err
		}

		log.Info("fixup database ok")
		return nil
	},
}

func startPProf(sec float64) {
	day := time.Now().Format("20060102")
	pprofCpuDir := path.Join(consts.GetDefaultYakitBaseTempDir(), "pprof", day, "cpu")
	err := os.MkdirAll(pprofCpuDir, 0o755)

	pprofMemDir := path.Join(consts.GetDefaultYakitBaseTempDir(), "pprof", day, "mem")
	err = os.MkdirAll(pprofMemDir, 0o755)
	if err != nil {
		log.Errorf("mkdir pprof dir failed: %s", err)
		return
	}
	for {
		// 启动 CPU 采样
		go func() {
			cpuFile, _ := os.Create(path.Join(pprofCpuDir, fmt.Sprintf("cpu_%d.pprof", time.Now().Unix())))
			defer cpuFile.Close()

			pprof.StartCPUProfile(cpuFile)
			time.Sleep(time.Duration(sec) * time.Second) // 采样 sec 秒
			pprof.StopCPUProfile()
		}()

		// 启动内存采样
		go func() {
			memFile, _ := os.Create(path.Join(pprofMemDir, fmt.Sprintf("mem_%d.pprof", time.Now().Unix())))
			defer memFile.Close()

			pprof.WriteHeapProfile(memFile)
		}()

		time.Sleep(time.Duration(sec) * time.Second) // 等待 sec 秒后再次采样
	}
}

func cliGroup(group string, cmds ...*cli.Command) []cli.Command {
	res := make([]cli.Command, len(cmds))
	for idx, i := range cmds {
		i.Category = group
		i.Hidden = false
		res[idx] = *i
	}
	return res
}

func main() {
	// log.SetLevel(log.WarnLevel)
	app := cli.NewApp()
	app.Usage = "yaklang core engine"
	app.Version = yakVersion
	app.IgnoreUnknownFlags = true
	consts.SetPalmVersion(yakVersion)
	consts.SetYakVersion(yakVersion)
	consts.SetYakBuildTime(buildTime)
	consts.SetYakGitHash(gitHash)

	// 启动 bridge
	tunnelServerCliApp := cybertunnel.GetTunnelServerCommandCli()
	tunnelServerCommand := cli.Command{
		Name:    "bridge",
		Usage:   "Create Yak-Bridge Server for OOB",
		Aliases: []string{"tunnel-server"},
		Flags:   tunnelServerCliApp.Flags,
		Before:  tunnelServerCliApp.Before,
		Action:  tunnelServerCliApp.Action,
	}

	mainCommands := []*cli.Command{
		yakcmds.LSPCommand,
		yakcmds.AIHTTPGatewayCommand,
		{
			Name: "version",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "json",
					Usage: "output as json",
				},
			},
			Usage: "Show Version Info",
			Action: func(c *cli.Context) {
				infoMap := map[string]string{"Version": yakVersion, "GoVersion": goVersion, "BuildTime": buildTime}
				if gitHash != "" {
					infoMap["GitHash"] = gitHash
				}
				if c.Bool("json") {
					b, err := json.Marshal(infoMap)
					if err != nil {
						log.Error(err)
						return
					}
					fmt.Printf("%s", b)
				} else {
					fmt.Println("Yak Language Build Info:")
					for k, v := range infoMap {
						fmt.Printf("    %v: %v\n", k, v)
					}
				}
			},
		},
		{
			Name:    "verify-cert",
			Aliases: []string{"vc"},
			Usage:   "Verify that the Yakit MITM certificate is in the system root certificate pool",
			Action: func(c *cli.Context) {
				if err := crep.VerifySystemCertificate(); err == nil {
					fmt.Println("mitm 证书在系统信任链中")
				} else {
					fmt.Println("mitm 证书不在系统信任链中!请重新安装证书")
				}
			},
		},

		{
			Name:  "compile",
			Usage: "Compile Yaklang Code to YakVM ByteCodes",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "output,o",
					Usage: "yakc的输出路径",
				},
				cli.StringFlag{
					Name:  "key,k",
					Usage: "编译yakc时所需要的密钥文件，是可选的，长度为128 bit(16 字节)，若提供了该密钥文件，后续执行yakc文件时，需要提供相同的密钥文件",
				},
			},
			Action: func(c *cli.Context) error {
				var (
					err error
					key []byte
				)

				if !yaklang.IsNew() {
					return utils.Errorf("compile command only support new engine")
				}

				args := c.Args()
				if len(args) <= 0 {
					return utils.Errorf("no source file")
				}

				keyfile := c.String("key")
				if keyfile != "" {
					key, err = ioutil.ReadFile(keyfile)
					if err != nil {
						return err
					}
				}

				file := args[0]
				outputFileName := c.String("output")
				if outputFileName == "" {
					oldExt := path.Ext(file)
					outputFileName = file[0:len(file)-len(oldExt)] + ".yakc"
				}

				if file == "" {
					return utils.Errorf("empty yak file")
				}

				raw, err := ioutil.ReadFile(file)
				if err != nil {
					return err
				}

				engine := yak.NewScriptEngine(100)
				err = engine.SetCryptoKey(key)
				if err != nil {
					return err
				}
				b, err := engine.Compile(string(raw))
				if err != nil {
					return err
				}
				err = ioutil.WriteFile(outputFileName, b, 0o644)
				if err != nil {
					return err
				}
				return nil
			},
		},
		&startGRPCServerCommand,
		&checkSecretLocalGRPCServerCommand,
		&fixupDatabaseCommand,
		&installSubCommand,
		&tunnelServerCommand,
		&mirrorGRPCServerCommand,
		&yakcmds.UpgradeCommand,
	}

	app.Commands = []cli.Command{}
	app.Commands = append(app.Commands, cliGroup("", mainCommands...)...)
	app.Commands = append(app.Commands, cliGroup("CVE Database Utils", yakcmds.CVEUtilCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Document Helper", yakcmds.DocCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Java Serialization Utils", yakcmds.JavaUtils...)...)
	app.Commands = append(app.Commands, cliGroup("Project Management", yakcmds.ProjectCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Traffic Utils", yakcmds.TrafficUtilCommands...)...)
	app.Commands = append(app.Commands, cliGroup("SSA Compiler", yakcmds.SSACompilerCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Utils", yakcmds.UtilsCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Network Distribution Utils", yakcmds.DistributionCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Vuln & Network Scanner", yakcmds.ScanCommands...)...)
	app.Commands = append(app.Commands, cliGroup("AI", yakcmds.AICommands...)...)
	app.Commands = append(app.Commands, cliGroup("Yak Ysoserial Util", yakcmds.YsoCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Git Utils", yakcmds.GitCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Systemd Service Management", yakcmds.SystemdCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Remote Operations", yakcmds.SSHCommands...)...)
	app.Commands = append(app.Commands, cliGroup("TUN Device Utils", yakcmds.TunCommands...)...)
	app.Commands = append(app.Commands, cliGroup("RAG Server", yakcmds.RAGServerCommands...)...)
	app.Commands = append(app.Commands, cliGroup("AI Viz Server", yakcmds.VizServerCommands...)...)
	app.Commands = append(app.Commands, cliGroup("Hot Patch Validators", yakcmds.HotPatchValidatorCommands...)...)
	app.Commands = append(app.Commands, *yakcmds.MemfitWorkerCommand)

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "code,c",
			Usage: "Yaklang Code",
		},
		cli.BoolFlag{
			Name:  "hex",
			Usage: "Hex Encoded Yak Code",
		},
		cli.BoolFlag{
			Name:  "base64",
			Usage: "Base64 Encoded Yak Code",
		},
		cli.StringFlag{
			Name:  "keyfile,k",
			Usage: "SecretKey-File for executing Yak Code, len: 128 bit(16 byte) PaddingFor PKCS7",
		},
		cli.StringFlag{
			Name:  "secret,s",
			Usage: "SecretKey for executing Yak Code, len: 128 bit(16 byte)",
		},
		cli.BoolFlag{
			Name:  "cdebug",
			Usage: "(Not Worked on Yakc) Enter Cli Debug Mode",
		},
		cli.StringFlag{
			Name:   "netx-proxy",
			Usage:  "Force Set Network Proxy for yak.netx",
			EnvVar: "NETX_PROXY",
		},
	}

	app.Action = func(c *cli.Context) error {
		if proxy := c.String("netx-proxy"); proxy != "" {
			netx.SetDefaultDialXConfig(netx.DialX_WithProxy(proxy))
		}
		var (
			err error
			key []byte
		)
		args := c.Args()
		keyfile := c.String("keyfile")
		debug := c.Bool("cdebug")

		setKey := false
		if keyfile != "" {
			p := utils.GetFirstExistedPath(keyfile)
			if p == "" {
				return utils.Errorf("keyfile not found: %s", keyfile)
			}

			key, err = ioutil.ReadFile(keyfile)
			if err != nil {
				return err
			}
			setKey = true
		} else if keyStr := c.String("secret"); keyStr != "" {
			key = []byte(keyStr)
			setKey = true
		}

		if setKey {
			if len(key) > 16 {
				key = key[:16]
			} else if len(key) == 16 {
				key = key[:]
			} else {
				key = codec.PKCS7Padding(key)
			}
		}

		if len(args) > 0 {
			// args 被解析到了，说明后面跟着文件，去读文件出来吧
			consts.SimpleYakGlobalConfig()
			file := args[0]
			if file != "" {
				absFile := file
				if !filepath.IsAbs(absFile) {
					absFile, err = filepath.Abs(absFile)
					if err != nil {
						return utils.Errorf("fetch abs file path failed: %s", err)
					}
				}
				raw, err := os.ReadFile(file)
				if err != nil {
					log.Errorf("read yak file[%s] failed: %s", file, err)
					if filepath.Ext(file) == "" {
						log.Infof("no file ext, maybe you want to execute an unexisted command? [%v]", file)
						fmt.Println("please check '--help' for more info.")
						fmt.Println()
					}
					return err
				}

				engine := yak.NewScriptEngine(100)
				// debug
				if debug {
					engine.SetDebug(debug)
					i := debugger.NewInteractiveDebugger()
					i.SetAbsFilePath(absFile)
					engine.SetDebugInit(i.Init())
					engine.SetDebugCallback(i.CallBack())
				}

				err = engine.SetCryptoKey(key)
				if err != nil {
					return err
				}
				err = engine.ExecuteMain(string(raw), absFile)
				if err != nil {
					return err
				}

				return nil
			} else {
				return utils.Errorf("empty yak file")
			}
		}

		code := c.String("code")
		if c.Bool("hex") {
			codeRaw, err := codec.DecodeHex(code)
			if err != nil {
				spew.Dump(code)
				return err
			}
			code = string(codeRaw)
		}

		if c.Bool("base64") {
			codeRaw, err := codec.DecodeBase64(code)
			if err != nil {
				spew.Dump(code)
				return err
			}
			code = string(codeRaw)
		}

		engine := yak.NewScriptEngine(100)
		err = engine.Execute(code)
		if err != nil {
			return err
		}

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
		return
	}
}

// grpcPhaseI18n maps engine startup phase identifiers to bilingual labels.
// These are the values emitted in the "phase" field of ready/failed events.
var grpcPhaseI18n = map[string]*schema.I18n{
	"init":         schema.NewI18n("初始化", "Initialization"),
	"pprof":        schema.NewI18n("性能分析服务启动", "pprof server startup"),
	"database":     schema.NewI18n("数据库初始化", "Database initialization"),
	"build_server": schema.NewI18n("gRPC 服务构建", "gRPC server build"),
	"cert":         schema.NewI18n("TLS 证书生成", "TLS certificate generation"),
	"listen":       schema.NewI18n("网络监听", "Network listen"),
	"serve":        schema.NewI18n("gRPC 服务运行", "gRPC server serve"),
	"wait_connect": schema.NewI18n("等待服务就绪", "Waiting for server ready"),
	"dial":         schema.NewI18n("gRPC 连接", "gRPC dial"),
	"version_rpc":  schema.NewI18n("认证与版本校验", "Authentication and Version RPC"),
}

// grpcReasonI18n maps structured reason codes to bilingual user-facing hints.
// Keys cover both:
//   - bracketed prefix codes from classifyListenError (e.g. tcp_bind_in_use)
//   - check-secret reason constants (e.g. database_error, dial_failed)
var grpcReasonI18n = map[string]*schema.I18n{
	ipcEndpointInvalid: schema.NewI18n(
		"Unix socket 的父路径未指向有效目录，请检查目录或符号链接的目标",
		"The Unix socket parent does not resolve to a directory. Check the directory or symbolic link target",
	),
	ipcBindDenied: schema.NewI18n(
		"无法创建本地 IPC 端点，请检查所选目录或命名管道的访问权限，或换一个可写位置",
		"Cannot create the local IPC endpoint. Check directory or named-pipe access permissions, or choose a writable location",
	),
	ipcBindInUse: schema.NewI18n(
		"本地 IPC 端点已存在，未覆盖或删除原端点；请选择其他 socket 路径或管道名",
		"The local IPC endpoint already exists and was left unchanged. Choose another socket path or pipe name",
	),
	ipcBindGeneric: schema.NewI18n(
		"本地 IPC 监听失败，请检查 socket 路径或管道名，以及所选位置的访问权限",
		"Local IPC listener failed. Check the socket path or pipe name and access permissions at the selected location",
	),
	// listen error sub-categories (from classifyListenError)
	"tcp_bind_denied": schema.NewI18n(
		"端口被系统策略阻止，请以管理员身份运行 Yakit，或检查防火墙设置",
		"Permission denied: the port may require elevated privileges or is blocked by system policy. Run Yakit as administrator, or check firewall settings",
	),
	"tcp_bind_in_use": schema.NewI18n(
		"端口被另一个进程占用，请结束占用该端口的旧进程，或切换到其他端口",
		"Address already in use: another process is listening on this port. Stop the process occupying this port, or switch to another port",
	),
	"tcp_bind_failed": schema.NewI18n(
		"网络监听失败，请检查端口配置或网络环境后重试",
		"Network listen failed. Check port configuration or network environment and retry",
	),
	"listen_failed": schema.NewI18n(
		"网络监听失败，请检查端口配置或网络环境后重试",
		"Network listen failed. Check port configuration or network environment and retry",
	),

	// check-secret reason constants
	"database_error": schema.NewI18n(
		"数据库初始化失败，可点击「修复数据库」按钮修复",
		"Database initialization failed. Click the Fix Database button to repair",
	),
	"build_server_failed": schema.NewI18n(
		"gRPC 服务构建失败，请重启 Yakit 后重试，如问题持续请重新安装引擎",
		"gRPC server build failed. Restart Yakit and retry; if the problem persists, reinstall the engine",
	),
	"dial_failed": schema.NewI18n(
		"连接 gRPC 服务器失败，请确认引擎进程正常运行，或重启 Yakit 后重试",
		"Failed to dial gRPC server. Ensure the engine process is running, or restart Yakit and retry",
	),
	"version_rpc_failed": schema.NewI18n(
		"认证或版本校验失败，请确认密码正确，或重启 Yakit 后重试",
		"Authentication or Version RPC failed. Verify the password is correct, or restart Yakit and retry",
	),
	"wait_connect_failed": schema.NewI18n(
		"等待 gRPC 服务就绪超时，请重启 Yakit 后重试",
		"Timed out waiting for gRPC server to be ready. Restart Yakit and retry",
	),

	// phase-fallback reason codes (used when no [xxx] prefix in error message)
	"init_failed": schema.NewI18n(
		"引擎初始化失败，请检查启动参数后重试",
		"Engine initialization failed. Check startup parameters and retry",
	),
	"pprof_failed": schema.NewI18n(
		"性能分析服务启动失败，请检查 pprof 端口配置后重试",
		"pprof server startup failed. Check pprof port configuration and retry",
	),
	"database_failed": schema.NewI18n(
		"数据库初始化失败，可点击「修复数据库」按钮修复",
		"Database initialization failed. Click the Fix Database button to repair",
	),
	"cert_failed": schema.NewI18n(
		"TLS 证书生成失败，请重启 Yakit 后重试，如问题持续请重新安装引擎",
		"TLS certificate generation failed. Restart Yakit and retry; if the problem persists, reinstall the engine",
	),
	"serve_failed": schema.NewI18n(
		"gRPC 服务运行异常，请重启 Yakit 后重试",
		"gRPC server serve failed. Restart Yakit and retry",
	),
	"unexpected_error": schema.NewI18n(
		"引擎发生意外错误，请重启 Yakit 后重试，如问题持续请重新安装引擎",
		"Engine encountered an unexpected error. Restart Yakit and retry; if the problem persists, reinstall the engine",
	),
}

// grpcEventPhaseI18n returns the bilingual label for a phase, or nil if unknown.
func grpcEventPhaseI18n(phase string) *schema.I18n {
	return grpcPhaseI18n[phase]
}

// grpcEventReasonI18n returns the bilingual hint for a reason code, or nil if unknown.
func grpcEventReasonI18n(reasonCode string) *schema.I18n {
	return grpcReasonI18n[reasonCode]
}
