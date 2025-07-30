package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
)

func main() {
	app := cli.NewApp()
	app.Name = "thirdparty-bin-manager"
	app.Usage = "Yaklang 第三方二进制工具管理器：安装、卸载和管理第三方工具"
	app.Version = "1.0.0"

	var (
		force      bool
		proxy      string
		timeout    int
		installDir string
		verbose    bool
	)

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:        "verbose",
			Usage:       "启用详细输出",
			Destination: &verbose,
		},
	}

	app.Before = func(c *cli.Context) error {
		if verbose {
			log.SetLevel(log.DebugLevel)
		}
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:  "list",
			Usage: "列出所有可用的二进制工具",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "builtin, b",
					Usage: "只显示内置工具",
				},
				cli.BoolFlag{
					Name:  "registered, r",
					Usage: "只显示已注册的工具",
				},
				cli.BoolFlag{
					Name:  "installed, i",
					Usage: "只显示已安装的工具",
				},
			},
			Action: func(c *cli.Context) error {
				showBuiltin := c.Bool("builtin")
				showRegistered := c.Bool("registered")
				showInstalled := c.Bool("installed")

				if !showBuiltin && !showRegistered && !showInstalled {
					// 默认显示所有
					showBuiltin = true
					showRegistered = true
				}

				if showBuiltin {
					fmt.Println("=== 内置二进制工具 ===")
					if err := thirdparty_bin.PrintBuiltinBinaries(); err != nil {
						log.Errorf("获取内置工具失败: %v", err)
					}
					fmt.Println()
				}

				if showRegistered {
					fmt.Println("=== 已注册的二进制工具 ===")
					binaries := thirdparty_bin.List()
					if len(binaries) == 0 {
						fmt.Println("没有已注册的工具")
					} else {
						for i, name := range binaries {
							fmt.Printf("%d. %s", i+1, name)
							if showInstalled {
								status, err := thirdparty_bin.GetStatus(name)
								if err == nil && status.Installed {
									fmt.Printf(" (已安装: %s)", status.InstallPath)
								} else {
									fmt.Printf(" (未安装)")
								}
							}
							fmt.Println()
						}
					}
					fmt.Println()
				}

				return nil
			},
		},
		{
			Name:      "install",
			Usage:     "安装指定的二进制工具",
			ArgsUsage: "<tool-name>",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:        "force, f",
					Usage:       "强制重新安装（覆盖已存在的文件）",
					Destination: &force,
				},
				cli.StringFlag{
					Name:        "proxy, p",
					Usage:       "使用代理下载 (http://proxy:port)",
					Destination: &proxy,
				},
				cli.IntFlag{
					Name:        "timeout, t",
					Usage:       "下载超时时间（秒）",
					Value:       300,
					Destination: &timeout,
				},
				cli.StringFlag{
					Name:        "install-dir, d",
					Usage:       "自定义安装目录",
					Destination: &installDir,
				},
			},
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("请指定要安装的工具名称")
				}

				toolName := c.Args().First()

				// 创建安装选项
				options := &thirdparty_bin.InstallOptions{
					Force: force,
					Proxy: proxy,
				}

				// 设置超时
				if timeout > 0 {
					ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
					defer cancel()
					options.Context = ctx
				} else {
					options.Context = context.Background()
				}

				// 设置进度回调
				options.Progress = func(progress float64, downloaded, total int64, message string) {
					if message == "开始下载" {
						log.Infof("安装包大小: %s", utils.ByteSize(uint64(total)))
						return
					}
					if total > 0 {
						fmt.Printf("\r下载进度: %.1f%% (%s/%s) - %s",
							progress*100,
							utils.ByteSize(uint64(downloaded)),
							utils.ByteSize(uint64(total)),
							message)
					} else {
						fmt.Printf("\r%s", message)
					}
				}

				fmt.Printf("正在安装 %s...\n", toolName)

				err := thirdparty_bin.Install(toolName, options)
				if err != nil {
					fmt.Printf("\n安装失败: %v\n", err)
					return err
				}

				fmt.Printf("\n✓ %s 安装成功\n", toolName)

				// 显示安装状态
				status, err := thirdparty_bin.GetStatus(toolName)
				if err == nil {
					fmt.Printf("安装路径: %s\n", status.InstallPath)
					if status.InstalledVersion != "" {
						fmt.Printf("版本: %s\n", status.InstalledVersion)
					}
				}

				return nil
			},
		},
		{
			Name:      "uninstall",
			Usage:     "卸载指定的二进制工具",
			ArgsUsage: "<tool-name>",
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("请指定要卸载的工具名称")
				}

				toolName := c.Args().First()

				fmt.Printf("正在卸载 %s...\n", toolName)

				err := thirdparty_bin.Uninstall(toolName)
				if err != nil {
					fmt.Printf("卸载失败: %v\n", err)
					return err
				}

				fmt.Printf("✓ %s 卸载成功\n", toolName)
				return nil
			},
		},
		{
			Name:      "status",
			Usage:     "查看指定工具的状态",
			ArgsUsage: "<tool-name>",
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("请指定要查看的工具名称")
				}

				toolName := c.Args().First()

				status, err := thirdparty_bin.GetStatus(toolName)
				if err != nil {
					fmt.Printf("获取状态失败: %v\n", err)
					return err
				}

				fmt.Printf("=== %s 状态 ===\n", toolName)
				fmt.Printf("名称: %s\n", status.Name)
				fmt.Printf("已安装: %v\n", status.Installed)
				if status.Installed {
					fmt.Printf("安装路径: %s\n", status.InstallPath)
					if status.InstalledVersion != "" {
						fmt.Printf("已安装版本: %s\n", status.InstalledVersion)
					}
				}
				if status.AvailableVersion != "" {
					fmt.Printf("可用版本: %s\n", status.AvailableVersion)
				}
				if status.NeedsUpdate {
					fmt.Printf("需要更新: 是\n")
				}

				return nil
			},
		},
		{
			Name:  "info",
			Usage: "显示系统和包信息",
			Action: func(c *cli.Context) error {
				// 显示包信息
				info := thirdparty_bin.GetPackageInfo()
				fmt.Println("=== 系统信息 ===")
				for key, value := range info {
					fmt.Printf("%s: %v\n", strings.ReplaceAll(key, "_", " "), value)
				}

				// 显示当前系统信息
				sysInfo := thirdparty_bin.GetCurrentSystemInfo()
				fmt.Printf("\n=== 当前系统 ===\n")
				fmt.Printf("操作系统: %s\n", sysInfo.OS)
				fmt.Printf("架构: %s\n", sysInfo.Arch)
				fmt.Printf("平台标识: %s\n", sysInfo.GetPlatformKey())

				return nil
			},
		},
		{
			Name:  "update",
			Usage: "更新所有已安装的工具",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "proxy, p",
					Usage:       "使用代理下载 (http://proxy:port)",
					Destination: &proxy,
				},
				cli.IntFlag{
					Name:        "timeout, t",
					Usage:       "下载超时时间（秒）",
					Value:       300,
					Destination: &timeout,
				},
			},
			Action: func(c *cli.Context) error {
				binaries := thirdparty_bin.List()
				if len(binaries) == 0 {
					fmt.Println("没有已注册的工具需要更新")
					return nil
				}

				fmt.Printf("检查 %d 个工具的更新...\n", len(binaries))

				updated := 0
				for _, name := range binaries {
					status, err := thirdparty_bin.GetStatus(name)
					if err != nil {
						log.Warnf("获取 %s 状态失败: %v", name, err)
						continue
					}

					if !status.Installed {
						continue
					}

					if status.NeedsUpdate {
						fmt.Printf("更新 %s...\n", name)

						options := &thirdparty_bin.InstallOptions{
							Force: true, // 强制更新
							Proxy: proxy,
						}

						if timeout > 0 {
							ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
							defer cancel()
							options.Context = ctx
						} else {
							options.Context = context.Background()
						}

						options.Progress = func(progress float64, downloaded, total int64, message string) {
							if total > 0 {
								fmt.Printf("\r  进度: %.1f%% (%s/%s)",
									progress*100,
									utils.ByteSize(uint64(downloaded)),
									utils.ByteSize(uint64(total)))
							}
						}

						err = thirdparty_bin.Install(name, options)
						if err != nil {
							fmt.Printf("\n  更新 %s 失败: %v\n", name, err)
						} else {
							fmt.Printf("\n  ✓ %s 更新成功\n", name)
							updated++
						}
					}
				}

				fmt.Printf("\n更新完成，共更新了 %d 个工具\n", updated)
				return nil
			},
		},
		{
			Name:  "reinit",
			Usage: "重新初始化内置二进制工具注册表",
			Action: func(c *cli.Context) error {
				fmt.Println("重新初始化内置二进制工具...")

				err := thirdparty_bin.ReinitializeBuiltinBinaries()
				if err != nil {
					fmt.Printf("重新初始化失败: %v\n", err)
					return err
				}

				fmt.Println("✓ 重新初始化成功")

				// 显示注册的工具
				binaries := thirdparty_bin.List()
				fmt.Printf("已注册 %d 个工具: %s\n", len(binaries), strings.Join(binaries, ", "))

				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("运行失败: %v", err)
		os.Exit(1)
	}
}
