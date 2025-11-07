package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/utils/grpc_recovery"

	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/yak/depinjector"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/yak/cmd/yakcmds"

	systemLog "log"

	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	_ "github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

func initializeDatabase(projectDatabase string, profileDBName string, frontendName string) error {
	// project and profile
	consts.InitializeYakitDatabase(projectDatabase, profileDBName, frontendName)

	// cve
	_, err := consts.InitializeCVEDatabase()
	if err != nil {
		log.Warnf("initialized cve database failed: %v", err)
	}

	if isVersionCommand() {
		return nil
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
		case "-v", "-version":
			return true
		default:
			return false
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
	log.Debugf("GC Percent Origin: %v -> %v", debug.SetGCPercent(GCPercentDefault), GCPercentDefault)
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

	if buildTime == "" {
		buildTime = time.Now().String()
	}

	if goVersion == "" {
		goVersion = runtime.Version()
	}

	/* 初始化数据库: 在 grpc 模式下，数据库应该不在 init 中使用 */
	if len(os.Args) > 1 && os.Args[1] == "grpc" {
		log.Debug("grpc should not initialize database in func:init")
		fmt.Printf(`
┓ ┳┳━┓┳┏ ┳  ┳━┓┏┓┓┏━┓
┗┏┛┃━┫┣┻┓┃  ┃━┫┃┃┃┃ ┳
 ┇ ┛ ┇┇ ┛┇━┛┛ ┇┇┗┛┇━┛
    %v %v

`, consts.GetYakVersion(), "yaklang.io")
	} else if len(os.Args) == 2 && (os.Args[1] == "-v" || os.Args[1] == "-version") {

	} else {
		err := initializeDatabase("", "", "")
		if err != nil {
			log.Warnf("initialize database failed: %s", err)
		}
	}
	yaklib.SetEngineInterface(yak.NewScriptEngine(1000))
	depinjector.DependencyInject()
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
		cli.Float64Flag{
			Name:  "auto-pprof",
			Usage: "指定 pprof 采集秒数间隔,eg. 10",
		},
		cli.BoolFlag{
			Name: "debug",
		},
		cli.StringFlag{
			Name:  "project-db",
			Usage: "Specific Project DB Name, eg. yakit-default.db",
		},
		cli.StringFlag{
			Name:  "profile-db",
			Usage: "Specific User-Data & Profile(Plugin) DB Name, eg yakit-profile-plugin.db",
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
			Usage: "关闭反连服务器",
		},
		cli.StringFlag{
			Name:  "common-name",
			Usage: "设置证书的 Common Name, 默认为 Server",
			Value: "Server",
		},
		cli.StringFlag{
			Name:  "local-password",
			Usage: "本地密码模式，使用固定端口 9011，不使用 TLS，与 host/port/secret/tls/gen-tls-crt 互斥",
		},
	},
	Action: func(c *cli.Context) error {
		// 检查 local-password 模式
		localPassword := c.String("local-password")
		localRandomPasswordPort := 9011
		if localPassword != "" {
			// 检查互斥选项
			if c.IsSet("host") && c.String("host") != "127.0.0.1" {
				return utils.Error("local-password is mutually exclusive with host option")
			}
			if c.IsSet("port") {
				manualPort := c.Int("port")
				if manualPort == 8087 {
					return utils.Error("local-password is mutually exclusive with port option")
				}
				if manualPort <= 0 {
					return utils.Error("local-password mode port must be positive")
				}
				localRandomPasswordPort = manualPort
			}
			if c.IsSet("secret") {
				return utils.Error("local-password is mutually exclusive with secret option")
			}
			if c.IsSet("tls") && c.Bool("tls") {
				return utils.Error("local-password is mutually exclusive with tls option")
			}
			if c.IsSet("gen-tls-crt") && c.String("gen-tls-crt") != "build/" {
				return utils.Error("local-password is mutually exclusive with gen-tls-crt option")
			}

			log.Info("starting grpc server in local-password mode")
			log.Infof("local-password mode: port=9011, tls=false, password=***")
		}

		if c.String("home") != "" {
			os.Setenv("YAKIT_HOME", c.String("home"))
		}
		if c.Bool("pprof") && c.IsSet("auto-pprof") {
			return utils.Error("Parameters 'pprof' and 'auto-pprof' cannot be set at the same time")
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
			runtime.SetBlockProfileRate(1)
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("---------------------------YAK GRPC PPROF-----------------------------")
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("USE: go tool pprof --seconds 30 http://127.0.0.1:18080/debug/pprof/profile")
			go func() {
				err := http.ListenAndServe(":18080", nil)
				if err != nil {
					return
				}
			}()
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

		frontendName := c.String("frontend")
		err := initializeDatabase(c.String("project-db"), c.String("profile-db"), frontendName)
		if err != nil {
			log.Errorf("init database failed: %s", err)
			return err
		}

		/* 初始化数据库后进行权限修复 */
		base := consts.GetDefaultYakitBaseDir()
		projectDatabaseName := consts.GetDefaultYakitProjectDatabase(base)
		profileDatabaseName := consts.GetDefaultYakitPluginDatabase(base)
		log.Infof("use project db: %s", projectDatabaseName)
		log.Infof("use profile db: %s", profileDatabaseName)

		yakit.TidyGeneralStorage(consts.GetGormProfileDatabase())

		certDir := c.String("gen-tls-crt")
		var caCertFile string = filepath.Join(certDir, "yakit-grpc-cert.pem")
		var caKeyFile string = filepath.Join(certDir, "yakit-grpc-key.pem")
		if certDir != "" {
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
		init_reverse := c.Bool("disable-reverse-server")
		s, err := yakgrpc.NewServer(
			yakgrpc.WithReverseServerPort(reverse_port),
			yakgrpc.WithInitFacadeServer(!init_reverse),
			yakgrpc.WithStartCacheLog(),
		)
		if err != nil {
			log.Errorf("build yakit server failed: %s", err)
			return err
		}
		ypb.RegisterYakServer(grpcTrans, s)

		// 确定监听地址和端口
		var host string
		var port int
		if localPassword != "" {
			// local-password 模式：强制使用 127.0.0.1:9011
			host = "127.0.0.1"
			port = localRandomPasswordPort
		} else {
			host = c.String("host")
			port = c.Int("port")
		}

		log.Infof("start to listen on: %v", utils.HostPort(host, port))
		var lis net.Listener

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
					return err
				}
				err = ioutil.WriteFile(caCertFile, cert, 0o600)
				if err != nil {
					return utils.Errorf("generate caCert[%s] failed: %s", caCertFile, err)
				}
				err = ioutil.WriteFile(caKeyFile, key, 0o600)
				if err != nil {
					return utils.Errorf("generate caKey[%s] failed: %s", caCertFile, err)
				}
			}

			if cert != nil {
				log.Infof("use current Root CA to login (For Yakit)\n\n%v\n\n", string(cert))
			}

			serverCert, serverKey, err := tlsutils.SignServerCrtNKeyWithParams(cert, key, cn, time.Now().Add(100*365*24*time.Hour), false)
			if err != nil {
				return err
			}

			tlsConfig, err := tlsutils.GetX509ServerTlsConfig(cert, serverCert, serverKey)
			if err != nil {
				return err
			}
			lis, err = tls.Listen("tcp", utils.HostPort(host, port), tlsConfig)
			if err != nil {
				log.Error(err)
				return err
			}
		} else {
			lis, err = net.Listen("tcp", utils.HostPort(host, port))
			if err != nil {
				log.Error(err)
				return err
			}
		}

		log.Infof("start to startup grpc server...")
		if host == "127.0.0.1" {
			if localPassword != "" {
				log.Info("the current yak grpc running in local-password mode on '127.0.0.1:9011'")
			} else {
				log.Info("the current yak grpc for '127.0.0.1', if u want to connect from other host. use \n" +
					"    yak grpc --host 0.0.0.0")
			}
		}
		log.Infof("yak grpc ok") // 勿删
		err = grpcTrans.Serve(lis)
		if err != nil {
			log.Error(err)
			return err
		}
		return nil
	},
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
	},
	Action: func(ctx *cli.Context) (finalError error) {
		var port = ctx.Int("port")

		addr := utils.HostPort("127.0.0.1", port)

		clientPassword := ctx.String("client-password")

		var version = consts.GetYakVersion()
		var secret = utils.RandStringBytes(16)

		defer func() {
			const extractorFlag = `50551aa97b5aa5ae8a3c3243ac60a8a7`

			if err := recover(); err != nil {
				finalError = utils.Errorf("panic: %v\n%s", err, spew.Sdump(string(debug.Stack())))
			}

			m := omap.NewGeneralOrderedMap()
			ok := utils.IsNil(finalError)
			var reason string
			if ok {
				reason = ""
			} else {
				reason = utils.InterfaceToString(finalError)
			}
			m.Set("ok", ok)
			m.Set("reason", reason)
			m.Set("host", "127.0.0.1")
			m.Set("port", port)
			m.Set("addr", utils.HostPort("127.0.0.1", port))
			m.Set("secret", secret)
			m.Set("version", version)
			result := string(m.Jsonify())
			fmt.Printf("\n<json-%v>\n%v\n</json-%v>\n\n", extractorFlag, result, extractorFlag)
		}()

		// 客户端模式：只连接服务器测试
		if clientPassword != "" {
			log.Info("running in client mode, connecting to existing server...")
			log.Infof("target: %s", addr)

			// 创建带超时的 context（10 秒）
			dialCtx, dialCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer dialCancel()

			// 创建客户端连接
			log.Infof("connecting to grpc server with password (timeout: 10s)...")
			conn, err := grpc.DialContext(
				dialCtx,
				addr,
				grpc.WithInsecure(),
				grpc.WithBlock(),
				grpc.WithDefaultCallOptions(
					grpc.MaxCallRecvMsgSize(100*1024*1024),
					grpc.MaxCallSendMsgSize(100*1024*1024),
				),
			)
			if err != nil {
				log.Errorf("failed to dial grpc server: %s", err)
				fmt.Printf("\n[FAILED] Cannot connect to server at %s\n", addr)
				fmt.Printf("Error: %s\n", err)
				fmt.Printf("Please check if:\n")
				fmt.Printf("  1. Server is running on port %d\n", port)
				fmt.Printf("  2. Local firewall is not blocking the connection\n")
				fmt.Printf("  3. Connection timeout (10s exceeded)\n\n")
				finalError = utils.Wrap(err, "dial grpc server failed")
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
				finalError = utils.Wrap(err, "call Version RPC failed")
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

		// 服务器模式：启动测试服务器
		log.Info("running in server mode, starting test server...")

		// 检查端口是否被占用
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			log.Errorf("failed to listen on port %d: %s", port, err)
			fmt.Printf("\n[FAILED] Port %d is already in use\n", port)
			fmt.Printf("Please check if:\n")
			fmt.Printf("  1. Local firewall is blocking the port\n")
			fmt.Printf("  2. Anti-virus software is preventing yak engine from starting\n")
			fmt.Printf("  3. Another yak instance is already running\n\n")
			err = utils.Wrapf(err, "port %d is occupied, please check local firewall or anti-virus software", port)
			finalError = utils.Wrap(err, "net.Listen(tcp, addr) failed")
			return
		}
		defer lis.Close()

		log.Infof("generated random secret for testing: %s", secret)

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
			finalError = utils.Wrap(err, "build yak grpc server failed")
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
		defer grpcTrans.Stop()

		// 等待服务器启动
		time.Sleep(time.Second)

		// 创建带超时的 context（10 秒）
		dialCtx, dialCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dialCancel()

		// 创建客户端连接
		log.Infof("connecting to test grpc server (timeout: 10s)...")
		conn, err := grpc.DialContext(
			dialCtx,
			addr,
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(100*1024*1024),
				grpc.MaxCallSendMsgSize(100*1024*1024),
			),
		)
		if err != nil {
			log.Errorf("failed to dial grpc server: %s", err)
			finalError = utils.Wrap(err, "dial grpc server failed")
			return
		}
		defer conn.Close()

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
			finalError = utils.Wrap(err, "call client.Version failed")
			return
		}

		log.Infof("Version RPC successful: %s", versionResp.Version)
		fmt.Printf("\n[SUCCESS] Local GRPC server with secret authentication test passed\n")
		fmt.Printf("  Port: %d\n", port)
		fmt.Printf("  Secret: %s\n", secret)
		fmt.Printf("  Version: %s\n\n", versionResp.Version)
		version = versionResp.Version
		finalError = nil
		return
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
	consts.SetPalmVersion(yakVersion)
	consts.SetYakVersion(yakVersion)

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
