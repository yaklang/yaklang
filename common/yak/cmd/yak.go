package main

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/utils/umask"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl"
	debugger "github.com/yaklang/yaklang/common/yak/interactive_debugger"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakdocument"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yserx"
	"github.com/yaklang/yaklang/scannode"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"

	// start pprof
	_ "net/http/pprof"
)

var (
	yakVersion string
	gitHash    string
	buildTime  string
	goVersion  string
)

func initializeDatabase(projectDatabase string, profileDBName string) error {
	consts.InitilizeDatabase(projectDatabase, profileDBName)
	_, err := consts.InitializeCVEDatabase()
	if err != nil {
		log.Infof("initialized cve database warning: %s", err)
	}

	// 这个顺序一般不要换
	consts.GetGormProjectDatabase().AutoMigrate(yakit.ProjectTables...)
	consts.GetGormProfileDatabase().AutoMigrate(yakit.ProfileTables...)

	// 调用一些数据库初始化的操作
	err = yakit.CallPostInitDatabase()
	if err != nil {
		return utils.Errorf("CallPostInitDatabase failed: %s", err)
	}
	return nil
}

func init() {
	// 取消掉 0022 的限制，让用户可以创建别人也能写的文件夹
	umask.Umask(0)

	/*
		进行一些必要初始化，永远不要再 init 中直接调用数据库，不然会破坏数据库加载的顺序
	*/
	log.Debugf(`Yaklang Engine %v Initializing`, yakVersion)

	os.Setenv("GODEBUG", "netdns=go")
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
	} else {
		err := initializeDatabase("", "")
		if err != nil {
			log.Warnf("initialize database failed: %s", err)
		}
	}
	yak.SetNaslExports(antlr4nasl.Exports)
	yak.InitYaklangLib()
}

var installSubCommand = cli.Command{
	Name:  "install",
	Usage: "安装 Yak/Install Yak  (Add to ENV PATH)",
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

var registeredTunnelOperators = []cli.Command{
	{
		Name:    "inspect-tuns",
		Usage:   "查看注册 tunnels 信息",
		Aliases: []string{"lst"},
		Flags: []cli.Flag{
			cli.StringFlag{Name: "server", Usage: "远程 Yak Bridge X 服务器", Value: "127.0.0.1:64333"},
			cli.StringFlag{Name: "secret", Usage: "远程 Yak Bridge X 服务器密码"},
			cli.StringFlag{Name: "secondary-password,x", Usage: "远程 Yak Bridge X 服务器的二级密码，避免别人查看注册管道"},
			cli.StringFlag{Name: "id", Usage: "指定 ID 查看 Tunnel 信息与认证"},
		},
		Action: func(c *cli.Context) error {
			ctx, client, _, err := cybertunnel.GetClient(context.Background(), c.String("server"), c.String("secret"))
			if err != nil {
				return err
			}

			showTunnel := func(tun *tpb.RegisterTunnelMeta) {
				withAuth, _ := client.GetRegisteredTunnelDescriptionByID(ctx, &tpb.GetRegisteredTunnelDescriptionByIDRequest{
					Id:                tun.GetId(),
					SecondaryPassword: c.String("secondary-password"),
				})
				fmt.Printf(`Tunnel: %v
	addr: %v
	note:
%v
	auth: 
%v
-----------------

`, tun.GetId(), utils.HostPort(tun.GetConnectHost(), tun.GetConnectPort()), tun.GetVerbose(), string(withAuth.GetAuth()))
			}

			id := c.String("id")
			if id != "" {
				rsp, err := client.GetRegisteredTunnelDescriptionByID(ctx, &tpb.GetRegisteredTunnelDescriptionByIDRequest{
					Id:                id,
					SecondaryPassword: c.String("secondary-password"),
				})
				if err != nil {
					return err
				}

				if len(rsp.GetAuth()) <= 0 {
					return utils.Errorf("cannot generate auth bytes for tun: %s", id)
				}

				showTunnel(rsp.GetInfo())
				println(string(rsp.GetAuth()))
				return nil
			}

			resp, err := client.GetAllRegisteredTunnel(ctx, &tpb.GetAllRegisteredTunnelRequest{
				SecondaryPassword: c.String("secondary-password"),
			})
			if err != nil {
				return err
			}
			for i := 0; i < len(resp.GetTunnels()); i++ {
				showTunnel(resp.Tunnels[i])
			}

			return nil
		},
	},
}

var mirrorGRPCServerCommand = cli.Command{
	Name:  "xgrpc",
	Usage: "启动 GRPC，开启映射",
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

var startGRPCServerCommand = cli.Command{
	Name:   "grpc",
	Usage:  "启动 GRPC 服务器",
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
			//Name: "port", Value: 8080,
			Usage: "启动 GRPC 的端口",
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
			Name: "pprof",
		},
		cli.StringFlag{
			Name:  "project-db",
			Usage: "Specific Project DB Name, eg. yakit-default.db",
		},
		cli.StringFlag{
			Name:  "profile-db",
			Usage: "Specific User-Data & Profile(Plugin) DB Name, eg yakit-profile-plugin.db",
		},
	},
	Action: func(c *cli.Context) error {
		enablePProfile := c.Bool("pprof")
		_ = enablePProfile
		if enablePProfile {
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("---------------------------YAK GRPC PPROF-----------------------------")
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			println("----------------------------------------------------------------------")
			go func() {
				err := http.ListenAndServe(":18080", nil)
				if err != nil {
					return
				}
			}()
		}
		log.SetLevel(log.DebugLevel)
		log.Info("start to initialize database")
		err := initializeDatabase(c.String("project-db"), c.String("profile-db"))
		if err != nil {
			log.Errorf("init database failed: %s", err)
			return err
		}

		/* 覆写核心插件 */
		//coreplugin.OverWriteCorePluginToLocal()

		/* 初始化数据库后进行权限修复 */
		base := consts.GetDefaultYakitBaseDir()
		var projectDatabaseName = consts.GetDefaultYakitProjectDatabase(base)
		var profileDatabaseName = consts.GetDefaultYakitPluginDatabase(base)
		log.Infof("use project db: %s", projectDatabaseName)
		log.Infof("use profile db: %s", profileDatabaseName)

		yakit.TidyGeneralStorage(consts.GetGormProfileDatabase())

		certDir := c.String("gen-tls-crt")
		var caCertFile string = filepath.Join(certDir, "yakit-grpc-cert.pem")
		var caKeyFile string = filepath.Join(certDir, "yakit-grpc-key.pem")
		if certDir != "" {
			err := os.MkdirAll(certDir, 0777)
			if err != nil {
				log.Warnf("mkdir certdir[%s] failed: %s", certDir, err)
			}
		}

		if c.String("home") != "" {
			os.Setenv("YAKIT_HOME", c.String("home"))
		}

		secret := c.String("secret")
		var streamInterceptors = []grpc.StreamServerInterceptor{grpc_recovery.StreamServerInterceptor()}
		var unaryInterceptors = []grpc.UnaryServerInterceptor{grpc_recovery.UnaryServerInterceptor()}
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

		log.Infof("start to create grpc schema...")
		grpcTrans := grpc.NewServer(
			grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
			grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
			grpc.MaxRecvMsgSize(100*1024*1024),
			grpc.MaxSendMsgSize(100*1024*1024),
		)
		s, err := yakgrpc.NewServer()
		if err != nil {
			log.Errorf("build yakit server failed: %s", err)
			return err
		}
		ypb.RegisterYakServer(grpcTrans, s)

		log.Infof("start to listen on: %v", utils.HostPort(c.String("host"), c.Int("port")))
		var lis net.Listener

		if c.Bool("tls") {
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
				cert, key, err = tlsutils.GenerateSelfSignedCertKeyWithCommonNameEx("Yakit TeamServer Root", "", nil, nil, nil, false)
				if err != nil {
					return err
				}
				err = ioutil.WriteFile(caCertFile, cert, 0600)
				if err != nil {
					return utils.Errorf("generate caCert[%s] failed: %s", caCertFile, err)
				}
				err = ioutil.WriteFile(caKeyFile, key, 0600)
				if err != nil {
					return utils.Errorf("generate caKey[%s] failed: %s", caCertFile, err)
				}
			}

			if cert != nil {
				log.Infof("use current Root CA to login (For Yakit)\n\n%v\n\n", string(cert))
			}

			serverCert, serverKey, err := tlsutils.SignServerCrtNKeyWithParams(cert, key, "Yakit TeamServer", time.Now().Add(100*365*24*time.Hour), false)
			if err != nil {
				return err
			}

			tlsConfig, err := tlsutils.GetX509ServerTlsConfig(cert, serverCert, serverKey)
			if err != nil {
				return err
			}
			lis, err = tls.Listen("tcp", utils.HostPort(c.String("host"), c.Int("port")), tlsConfig)
			if err != nil {
				log.Error(err)
				return err
			}
		} else {
			lis, err = net.Listen("tcp", utils.HostPort(c.String("host"), c.Int("port")))
			if err != nil {
				log.Error(err)
				return err
			}
		}

		log.Infof("start to startup grpc server...")
		if c.String("host") == "127.0.0.1" {
			log.Info("the current yak grpc for '127.0.0.1', if u want to connect from other host. use \n" +
				"    yak grpc --host 0.0.0.0")
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

var distYakCommand = scannode.DistYakCommand

var mqConnectCommand = cli.Command{
	Name:   "mq",
	Usage:  "distributed by private amqp application protocol, execute yak via rabbitmq",
	Before: nil,
	After:  nil,
	Action: func(c *cli.Context) error {
		config := spec.LoadAMQPConfigFromCliContext(c)
		node, err := scannode.NewScanNode(c.String("id"), c.String("server-port"), config)
		if err != nil {
			return err
		}
		node.Run()
		return nil
	},
	Flags: spec.GetCliBasicConfig("scannode"),
}

var cveCommand = cli.Command{
	Name: "build-cve-database",
	Flags: []cli.Flag{
		cli.BoolFlag{Name: "cwe"},
		cli.BoolFlag{Name: "cache"},
		cli.StringFlag{Name: "output,o"},
		cli.StringFlag{Name: "description-db"},
		cli.IntFlag{Name: "year"},
		cli.BoolFlag{Name: "no-gzip"},
	},
	Action: func(c *cli.Context) error {
		cvePath := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "cve")
		os.MkdirAll(cvePath, 0755)

		/* 开始构建 */
		var outputFile = c.String("output")
		if outputFile == "" {
			outputFile = consts.GetCVEDatabasePath()
		}
		outputDB, err := gorm.Open("sqlite3", outputFile)
		if err != nil {
			return err
		}
		outputDB.AutoMigrate(&cveresources.CVE{}, &cveresources.CWE{})
		gzipHandler := func() error {
			if c.Bool("no-gzip") {
				return nil
			}
			log.Infof("start to zip... %v", outputFile)
			zipFile := outputFile + ".gzip"
			fp, err := os.OpenFile(zipFile, os.O_CREATE|os.O_RDWR, 0644)
			if err != nil {
				return err
			}
			defer fp.Close()

			w := gzip.NewWriter(fp)
			srcFp, err := os.Open(outputFile)
			if err != nil {
				return err
			}
			io.Copy(w, srcFp)
			srcFp.Close()
			w.Flush()
			w.Close()
			return nil
		}

		descDBPath := c.String("description-db")
		log.Infof("description-db: %v", descDBPath)
		if descDBPath == "" {
			_, _ = consts.InitializeCVEDescriptionDatabase()
			descDBPath = consts.GetCVEDescriptionDatabasePath()
		}
		descDB, err := gorm.Open("sqlite3", descDBPath)
		if err != nil {
			log.Warnf("cannot found sqlite3 cve description: %v", err)
		}

		if c.Bool("cwe") {
			cveDB := outputDB
			if descDB != nil && descDB.HasTable("cwes") && cveDB != nil {
				log.Info("cve-description database is detected, merge cve db")
				if cveDB.HasTable("cwes") {
					if db := cveDB.DropTable("cwes"); db.Error != nil {
						log.Errorf("drop cwe table failed: %s", db.Error)
					}
				}
				log.Infof("start to migrate cwe for cvedb")
				cveDB.AutoMigrate(&cveresources.CWE{})
				for cwe := range cveresources.YieldCWEs(descDB.Model(&cveresources.CVE{}), context.Background()) {
					cveresources.CreateOrUpdateCWE(cveDB, cwe.IdStr, cwe)
				}
				return gzipHandler()
			}

			log.Info("start to download cwe")
			fp, err := cvequeryops.DownloadCWE()
			if err != nil {
				return err
			}
			log.Info("start to load cwes")
			cwes, err := cvequeryops.LoadCWE(fp)
			if err != nil {
				return err
			}
			log.Infof("total cwes: %v", len(cwes))
			db := cveDB
			db.AutoMigrate(&cveresources.CWE{})
			cvequeryops.SaveCWE(db, cwes)
			return gzipHandler()
		}

		wg := new(sync.WaitGroup)
		wg.Add(2)
		var downloadFailed bool
		go func() {
			defer wg.Done()
			log.Infof("start to save cve data from database: %v", cvePath)
			if !c.Bool("cache") {
				err := cvequeryops.DownLoad(cvePath)
				if err != nil {
					log.Error("download failed: %s, err")
					downloadFailed = true
					return
				}
			}

		}()
		go func() {
			defer wg.Done()

			log.Infof("using description database: %s", descDBPath)
			db, err := gorm.Open("sqlite3", descDBPath)
			if err != nil {
				log.Error("sqlite3 failed: %s", err)
				return
			}
			log.Info("start to handling cve description db")
			var v = make(map[string]cveresources.CVEDesc)
			var count int
			for i := range cve.YieldCVEDescriptions(db, context.Background()) {
				count++
				//_, ok := v[i.CVE]
				//if ok {
				//	panic("existed cache " + i.CVE)
				//}
				v[i.CVE] = cveresources.CVEDesc{
					TitleZh:           i.ChineseTitle,
					Solution:          i.OpenAISolution,
					DescriptionMainZh: i.ChineseDescription,
				}
			}
			cveresources.RegisterDesc(v)
			log.Infof("register description finished! total: %v", count)
		}()

		wg.Wait()
		if downloadFailed {
			return utils.Error("download failed")
		}

		var years []int
		if ret := c.Int("year"); ret > 0 {
			years = append(years, ret)
		}
		cvequeryops.LoadCVE(cvePath, outputFile, years...)
		return gzipHandler()
	},
}

var translatingCommand = cli.Command{
	Name: "translating",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "keyfile",
			Usage: "API Key 的文件",
		},
		cli.BoolFlag{
			Name: "no-critical",
		},
		cli.IntFlag{
			Name:  "concurrent",
			Value: 10,
		},
		cli.StringFlag{
			Name: "cve-database",
		},
		cli.BoolFlag{
			Name: "cwe",
		},
		cli.BoolFlag{
			Name: "chaosmaker-rules",
		},
	},
	Hidden: true,
	Action: func(c *cli.Context) error {
		if c.Bool("chaosmaker-rules") {
			chaosmaker.DecorateRules(c.Int("concurrent"), "http://127.0.0.1:7890")
			return nil
		}

		if c.Bool("cwe") {
			return cve.TranslatingCWE(c.String("keyfile"), c.Int("concurrent"), c.String("cve-database"))
		}
		return cve.Translating(c.String("keyfile"), c.Bool("no-critical"), c.Int("concurrent"), c.String("cve-database"))
	},
}

func main() {
	//log.SetLevel(log.DebugLevel)
	app := cli.NewApp()
	app.Usage = "yaklang core engine"
	app.Version = yakVersion
	consts.SetPalmVersion(yakVersion)
	consts.SetYakVersion(yakVersion)

	// 启动 bridge
	tunnelServerCliApp := cybertunnel.GetTunnelServerCommandCli()
	tunnelServerCommand := cli.Command{
		Name:    "tunnel-server",
		Aliases: []string{"bridge"},
		Flags:   tunnelServerCliApp.Flags,
		Before:  tunnelServerCliApp.Before,
		Action:  tunnelServerCliApp.Action,
	}

	app.Commands = []cli.Command{
		{Name: "gzip", Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "f,file",
				Usage: "input file",
			},
			cli.BoolFlag{Name: "d,decode"},
			cli.StringFlag{Name: "o,output"},
		}, Action: func(c *cli.Context) error {
			f := c.String("file")
			if utils.GetFirstExistedFile(f) == "" {
				return utils.Errorf("non-existed: %v", f)
			}
			originFp, err := os.Open(f)
			if err != nil {
				return err
			}
			defer originFp.Close()

			if c.Bool("decode") {
				outFile := c.String("output")
				if outFile == "" {
					return utils.Error("decode need output not empty")
				}
				log.Infof("start to d-gzip to %v", outFile)
				targetFp, err := os.OpenFile(outFile, os.O_CREATE|os.O_RDWR, 0666)
				if err != nil {
					return err
				}
				defer targetFp.Close()
				r, err := gzip.NewReader(originFp)
				if err != nil {
					return err
				}
				defer r.Close()
				io.Copy(targetFp, r)
				log.Infof("finished")
				return nil
			}

			gf := f + ".gzip"
			fp, err := os.OpenFile(gf, os.O_CREATE|os.O_RDWR, 0666)
			if err != nil {
				return err
			}
			defer fp.Close()
			gzipWriter := gzip.NewWriter(fp)
			io.Copy(gzipWriter, originFp)
			gzipWriter.Flush()
			gzipWriter.Close()
			return nil
		}},
		{Name: "hex", Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "f,file",
				Usage: "input file",
			},
			cli.StringFlag{
				Name:  "d,data",
				Usage: "input data",
			},
		}, Action: func(c *cli.Context) {
			if c.String("file") != "" {
				raw, err := ioutil.ReadFile(c.String("file"))
				if err != nil {
					log.Error(err)
					return
				}
				println(codec.EncodeToHex(raw))
			}

			if c.String("data") != "" {
				println(codec.EncodeToHex(c.String("data")))
			}
		}},
		{Name: "serialdumper", Aliases: []string{"sd"}, Action: func(c *cli.Context) {
			if len(c.Args()) > 0 {
				raw, err := codec.DecodeHex(c.Args()[0])
				if err != nil {
					log.Error(err)
					return
				}
				d := yserx.JavaSerializedDumper(raw)
				println(d)
			}
		}},
		{Name: "version", Action: func(c *cli.Context) {
			if gitHash != "" {
				fmt.Printf(`Yak Language Build Info:
    Version: %v-%v
    GoVersion: %v
    GitHash: %v
    BuildTime: %v

`, yakVersion, gitHash, goVersion, gitHash, buildTime)
			} else {
				fmt.Printf(`Yak Language Build Info:
    Version: %v
    GoVersion: %v
    BuildTime: %v

`, yakVersion, goVersion, buildTime)
			}
		}},
		{
			Name: "tunnel",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "server", Value: "cybertunnel.run:64333"},
				cli.IntFlag{Name: "local-port", Value: 53},
				cli.StringFlag{Name: "local-host", Value: "127.0.0.1"},
				cli.IntFlag{Name: "remote-port", Value: 53},
				cli.StringFlag{Name: "secret", Value: ""},
				cli.StringFlag{Name: "network,proto", Value: "tcp"},
			},
			Action: func(c *cli.Context) error {
				return cybertunnel.MirrorLocalPortToRemoteEx(
					c.String("network"),
					c.String("local-host"),
					c.Int("local-port"),
					c.Int("remote-port"),
					"test-cli",
					c.String("server"),
					c.String("secret"),
					context.Background(),
				)
			},
		},
		{
			Name: "gendoc",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dir",
					Usage: "生成的文档路径",
					Value: "docs",
				},
			},
			Action: func(c *cli.Context) error {
				libs := yak.EngineToLibDocuments(yaklang.New())
				baseDir := filepath.Join(".", c.String("dir"))

				_ = os.MkdirAll(baseDir, 0777)
				for _, lib := range libs {
					targetFile := filepath.Join(baseDir, fmt.Sprintf("%v.yakdoc.yaml", lib.Name))
					existed := yakdocument.LibDoc{}
					if utils.GetFirstExistedPath(targetFile) != "" {
						raw, _ := ioutil.ReadFile(targetFile)
						_ = yaml.Unmarshal(raw, &existed)
					}

					lib.Merge(&existed)
					raw, _ := yaml.Marshal(lib)
					_ = ioutil.WriteFile(targetFile, raw, os.ModePerm)
				}

				for _, s := range yakdocument.LibsToRelativeStructs(libs...) {
					targetFile := filepath.Join(baseDir, "structs", fmt.Sprintf("%v.struct.yakdoc.yaml", s.StructName))
					dir, _ := filepath.Split(targetFile)
					_ = os.MkdirAll(dir, 0777)
					existed := yakdocument.StructDocForYamlMarshal{}
					if utils.GetFirstExistedPath(targetFile) != "" {
						raw, err := ioutil.ReadFile(targetFile)
						if err != nil {
							log.Errorf("cannot find file[%s]: %s", targetFile, err)
							continue
						}
						err = yaml.Unmarshal(raw, &existed)
						if err != nil {
							log.Errorf("unmarshal[%s] failed: %s", targetFile, err)
						}
					}

					if existed.StructName != "" {
						s.Merge(&existed)
					}
					raw, _ := yaml.Marshal(s)
					_ = ioutil.WriteFile(targetFile, raw, os.ModePerm)
				}
				return nil
			},
		},
		{
			Name: "builddoc",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "from",
					Usage: "生成的文档源文件路径",
					Value: "docs",
				},
				cli.StringFlag{
					Name:  "to",
					Usage: "生成 Markdown 内容",
					Value: "build/yakapis/",
				},
				cli.StringFlag{
					Name:  "to-vscode-data,tovd",
					Value: "build/yaklang-completion.json",
				},
			},
			Action: func(c *cli.Context) error {
				libs := yak.EngineToLibDocuments(yaklang.New())
				baseDir := filepath.Join(".", c.String("from"))

				outputDir := filepath.Join(".", c.String("to"))
				_ = os.MkdirAll(outputDir, os.ModePerm)

				_ = os.MkdirAll(baseDir, os.ModePerm)
				for _, lib := range libs {
					targetFile := filepath.Join(baseDir, fmt.Sprintf("%v.yakdoc.yaml", lib.Name))
					existed := yakdocument.LibDoc{}
					if utils.GetFirstExistedPath(targetFile) != "" {
						raw, _ := ioutil.ReadFile(targetFile)
						_ = yaml.Unmarshal(raw, &existed)
					}

					lib.Merge(&existed)

					outputFileName := filepath.Join(outputDir, fmt.Sprintf("%v.md", strings.ReplaceAll(lib.Name, ".", "_")))
					_ = outputFileName

					results := lib.ToMarkdown()
					if results == "" {
						return utils.Errorf("markdown empty... for %v", lib.Name)
					}
					err := ioutil.WriteFile(outputFileName, []byte(results), os.ModePerm)
					if err != nil {
						return err
					}
				}

				completionJsonRaw, err := yakdocument.LibDocsToCompletionJson(libs...)
				if err != nil {
					return err
				}
				err = ioutil.WriteFile(c.String("to-vscode-data"), completionJsonRaw, os.ModePerm)
				if err != nil {
					return utils.Errorf("write vscode auto-completions json failed: %s", err)
				}
				return nil
			},
		},
		{
			Name:  "doc",
			Usage: "查看脚本引擎所有的可使用的接口和说明",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "lib,extlib,l,t",
					Usage: "展示特定第三方扩展包的定义和帮助信息",
				},
				cli.StringFlag{
					Name:  "func,f",
					Usage: "展示特定第三方扩展包函数的定义",
				},
				cli.BoolFlag{
					Name:  "all-lib,all-libs,libs",
					Usage: "展示所有第三方包的帮助信息",
				},
			},
			Action: func(c *cli.Context) error {
				helper := doc.Document

				if c.Bool("all-lib") {
					for _, libName := range helper.GetAllLibs() {
						helper.ShowLibHelpInfo(libName)
					}
					return nil
				}

				extLib := c.String("extlib")
				function := c.String("func")
				if extLib == "" && function != "" {
					extLib = "__GLOBAL__"
				}

				if extLib == "" {
					helper.ShowHelpInfo()
					return nil
				}

				if function != "" {
					if info := helper.LibFuncHelpInfo(extLib, function); info == "" {
						log.Errorf("palm script engine no such function in %s: %v", extLib, function)
						return nil
					} else {
						helper.ShowLibFuncHelpInfo(extLib, function)
					}
				} else {
					if info := helper.LibHelpInfo(extLib); info == "" {
						log.Errorf("palm script engine no such extlib: %v", extLib)
						return nil
					} else {
						helper.ShowLibHelpInfo(extLib)
					}
				}

				return nil
			},
		},
		{
			Name:  "compile",
			Usage: "编译yak脚本，生成yakc文件(仅限新引擎)",
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
				err = ioutil.WriteFile(outputFileName, b, 0644)
				if err != nil {
					return err
				}
				return nil
			},
		},
		{Hidden: true, Name: "vscdoc", Action: func(c *cli.Context) error {
			libs := yak.EngineToLibDocuments(
				yaklang.New(),
			)
			_ = libs
			return nil
		},
		},
		{Name: "profile-export", Action: func(c *cli.Context) {
			f := c.String("output")
			if utils.GetFirstExistedPath(f) != "" {
				log.Errorf("path[%s] is existed", f)
				return
			}

			if c.String("type") == "" {
				log.Error("export type cannot be emtpy")
				return
			}
			switch ret := strings.ToLower(c.String("type")); ret {
			case "plugin", "plugins":
				err := yakit.ExportYakScript(consts.GetGormProfileDatabase(), f)
				if err != nil {
					log.Error("output failed: %s", err)
				}
			default:
				log.Error("unsupported resource type: " + ret)
				return
			}
		}, Flags: []cli.Flag{
			cli.StringFlag{Name: "output"},
			cli.StringFlag{Name: "type"},
		}},

		installSubCommand,
		startGRPCServerCommand,
		tunnelServerCommand,

		mirrorGRPCServerCommand,
		registeredTunnelOperators[0],

		// 分布式用到的两个命令
		mqConnectCommand,
		distYakCommand,

		// CVE 相关命令
		cveCommand,
		translatingCommand,
	}
	app.Commands = append(app.Commands, yak.Subcommands...)

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "code,c",
			Usage: "yak代码",
		},
		cli.BoolFlag{
			Name:  "hex",
			Usage: "yak代码(被HEX编码)",
		},
		cli.BoolFlag{
			Name:  "base64",
			Usage: "yak代码(被Base64编码)",
		},
		cli.StringFlag{
			Name:  "key,k",
			Usage: "执行yakc时所需要的密钥文件，是可选的，长度为128 bit(16 字节)",
		},
		cli.BoolFlag{
			Name:  "cdebug",
			Usage: "以命令行debug模式执行yak(仅限新引擎,对yakc文件无效)，进入cli debug",
		},
	}

	app.Action = func(c *cli.Context) error {
		var (
			err error
			key []byte
		)
		args := c.Args()
		keyfile := c.String("key")
		debug := c.Bool("cdebug")
		if keyfile != "" {
			key, err = ioutil.ReadFile(keyfile)
			if err != nil {
				return err
			}
		}

		if len(args) > 0 {
			// args 被解析到了，说明后面跟着文件，去读文件出来吧
			file := args[0]
			if file != "" {
				var absFile = file
				if !filepath.IsAbs(absFile) {
					absFile, err = filepath.Abs(absFile)
					if err != nil {
						return utils.Errorf("fetch abs file path failed: %s", err)
					}
				}
				raw, err := ioutil.ReadFile(file)
				if err != nil {
					return err
				}

				engine := yak.NewScriptEngine(100)
				// debug
				if debug {
					engine.SetDebug(debug)
					i := debugger.NewInteractiveDebugger()
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
			var codeRaw, err = codec.DecodeHex(code)
			if err != nil {
				spew.Dump(code)
				return err
			}
			code = string(codeRaw)
		}

		if c.Bool("base64") {
			var codeRaw, err = codec.DecodeBase64(code)
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
		println(err.Error())
		return
	}
}
