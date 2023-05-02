package cybertunnel

import (
	"context"
	"net"
	"os"
	"syscall"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"google.golang.org/grpc"

	"github.com/yaklang/yaklang/common/cybertunnel/dnslog"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func GetTunnelServerCommandCli() *cli.App {
	app := cli.NewApp()

	/* setting log */
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "log-level,l",
			Value: "info",
		},
		cli.BoolFlag{
			Name: "quiet,q",
		},
		cli.StringFlag{
			Name:  "addr",
			Value: "0.0.0.0:64333",
		},

		cli.BoolFlag{
			Name:  "debug",
			Usage: "Debug mode for see database output",
		},

		cli.DurationFlag{
			Name:  "retry-timeout",
			Usage: "Set retry timeout",
			Value: 10 * time.Second,
		},

		cli.StringFlag{
			Name:  "secret",
			Value: "",
		},

		cli.BoolFlag{
			Name: "dnslog",
		},

		cli.StringFlag{
			Name:  "domain",
			Usage: "Set DNSLog RootDomain",
		},

		cli.StringFlag{
			Name:  "public-ip",
			Usage: "Public IP Address: Set the public IP address",
		},

		cli.StringFlag{
			Name: "secondary-password,x", Hidden: true,
			EnvVar: "YAK_BRIDGE_SECONDARY_PASSWORD",
			Usage:  "Secondary password for remote Yak Bridge server to prevent others from viewing the registration pipeline",
		},
	}

	app.Commands = []cli.Command{
		{
			Name: "remote-ip",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "server",
					Value: "127.0.0.1:64333",
				},
			},
			Action: func(c *cli.Context) error {
				i, err := GetTunnelServerExternalIP(c.String("server"), c.String("secret"))
				if err != nil {
					return err
				}
				println(i.String())
				return nil
			},
		},
		{
			Name: "mirror",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "server", Value: "127.0.0.1:64333"},
				cli.IntFlag{Name: "local-port", Value: 8843},
				cli.IntFlag{Name: "remote-port", Value: 8844},
				cli.StringFlag{Name: "secret", Value: ""},
				cli.StringFlag{Name: "network,proto", Value: "tcp"},
			},
			Action: func(c *cli.Context) error {
				swg := utils.NewSizedWaitGroup(2)
				swg.Add()
				go func() {
					defer swg.Done()
					for {
						err := MirrorLocalPortToRemote(
							c.String("network"),
							c.Int("local-port"),
							c.Int("remote-port"),
							"test-cli",
							c.String("server"),
							c.String("secret"),
							context.Background(),
						)
						time.Sleep(time.Second)
						if err != nil {
							log.Errorf("mirror error: %s", err)
						}
					}
				}()
				swg.Wait()
				return nil
			},
		},
	}

	app.Before = func(context *cli.Context) error {
		if context.Bool("quiet") {
			return nil
		}

		level, err := log.ParseLevel(context.String("log-level"))
		if err != nil {
			return errors.Errorf("failed to parse %s as log level", context.String("log-level"))
		}

		log.SetLevel(level)
		return nil
	}

	app.Action = func(c *cli.Context) error {
		var err error

		addr := c.String("addr")
		host, port, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			return err
		}

		ticker := time.Tick(1 * time.Second)
		timer := time.NewTimer(c.Duration("retry-timeout"))

		var (
			lis net.Listener
		)

	RETRYING_LISTEN:
		for {
			select {
			case <-ticker:
				log.Infof("start to listen: %s", utils.HostPort(host, port))
				lis, err = net.Listen("tcp", utils.HostPort(host, port))
				if err != nil {
					log.Errorf("retry to dial: %s, failed: %s", addr, err)
					continue
				}
				break RETRYING_LISTEN
			case <-timer.C:
				break RETRYING_LISTEN
			}
		}
		if lis == nil {
			lis, err = net.Listen("tcp", addr)
			if err != nil {
				return errors.Errorf("dail %s failed: %s", addr, err)
			}
		}
		defer func() {
			_ = lis.Close()
		}()

		secret := c.String("secret")
		var streamInterceptors = []grpc.StreamServerInterceptor{grpc_recovery.StreamServerInterceptor()}
		var unaryInterceptors = []grpc.UnaryServerInterceptor{grpc_recovery.UnaryServerInterceptor()}
		if secret != "" && !c.Bool("dnslog") {
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
			var authStreamInterceptor grpc.StreamServerInterceptor = func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				return grpc_auth.StreamServerInterceptor(auth)(srv, ss, info, handler)
			}
			var authUnaryInterceptor grpc.UnaryServerInterceptor = func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				return grpc_auth.UnaryServerInterceptor(auth)(ctx, req, info, handler)
			}
			//streamInterceptors = append(streamInterceptors, grpc_auth.StreamServerInterceptor(auth))
			streamInterceptors = append(streamInterceptors, authStreamInterceptor)
			unaryInterceptors = append(unaryInterceptors, authUnaryInterceptor)
		}

		log.Infof("start to create grpc schema...")
		grpcTrans := grpc.NewServer(
			grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
			grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
			grpc.MaxRecvMsgSize(100*1024*1024),
			grpc.MaxSendMsgSize(100*1024*1024),
		)
		s, err := NewTunnelServer()
		if err != nil {
			log.Errorf("build tunnel server failed: %s", err)
			return err
		}
		s.ExternalIP = c.String("public-ip")
		s.SecondaryPassword = c.String("secondary-password")
		if s.SecondaryPassword == "" {
			err := s.InitialReverseTrigger()
			if err != nil {
				return utils.Errorf("initial reverse trigger failed: %s", err)
			}
		}

		if c.Bool("dnslog") {
			if c.String("domain") == "" {
				return utils.Error("empty dnslog domain config")
			}
			dnslogServer, err := dnslog.NewDNSLogServer(c.String("domain"), c.String("public-ip"))
			if err != nil {
				return utils.Errorf("serve dns log failed: %s", err)
			}
			tpb.RegisterDNSLogServer(grpcTrans, dnslogServer)
		}
		//else {
		//	tpb.RegisterTunnelServer(grpcTrans, s)
		//}
		tpb.RegisterTunnelServer(grpcTrans, s)

		go func() {
			sigC := utils.NewSignalChannel(syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
			for {
				select {
				case <-sigC:
					os.Exit(1)
				}
			}
		}()

		for {
			log.Infof("serve grpc tunnel at %v", lis.Addr().String())
			err = grpcTrans.Serve(lis)
			if err != nil {
				log.Errorf("failed to serve: %s try again...", err)
				time.Sleep(1 * time.Second)
			}
		}
	}

	return app
}
