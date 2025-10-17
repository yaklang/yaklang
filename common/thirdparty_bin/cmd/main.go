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

// renderProgressBar 渲染一个美观的进度条
func renderProgressBar(progress float64, downloaded, total int64, message string) string {
	const (
		barWidth  = 40
		fullChar  = "█"
		emptyChar = "░"
	)

	// 确保进度在0-1之间
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	// 计算填充的字符数
	filled := int(progress * barWidth)

	// 构建进度条
	bar := strings.Repeat(fullChar, filled) + strings.Repeat(emptyChar, barWidth-filled)

	// 格式化显示
	var result strings.Builder

	if total > 0 {
		percentage := progress * 100
		downloadedSize := utils.ByteSize(uint64(downloaded))
		totalSize := utils.ByteSize(uint64(total))

		// 彩色进度条（使用ANSI颜色代码）
		var barColor string
		if percentage < 30 {
			barColor = "\033[31m" // 红色
		} else if percentage < 70 {
			barColor = "\033[33m" // 黄色
		} else {
			barColor = "\033[32m" // 绿色
		}

		result.WriteString(fmt.Sprintf("%s[%s]\033[0m %6.1f%% %s/%s",
			barColor,
			bar,
			percentage,
			downloadedSize,
			totalSize))

		if message != "" && message != "下载中" {
			result.WriteString(fmt.Sprintf(" - %s", message))
		}
	}

	return result.String()
}

// clearProgressLine 清除进度条行
func clearProgressLine() {
	// 使用ANSI转义序列清除整行
	fmt.Print("\033[2K\r")
}

// showAllRegisteredTools 显示所有注册工具的状态和描述信息
func showAllRegisteredTools() error {
	fmt.Println("=== Registered Binary Tools ===")

	binaries := thirdparty_bin.ListRegisteredNames()
	if len(binaries) == 0 {
		fmt.Println("   No tools registered")
		fmt.Println("   Use 'reinit' to reload builtin tools")
		return nil
	}

	fmt.Printf("Total tools: %d\n\n", len(binaries))

	for i, name := range binaries {
		status, err := thirdparty_bin.GetStatus(name)
		if err != nil {
			fmt.Printf("   %d. %s (Error: %v)\n", i+1, name, err)
			continue
		}

		// 获取工具描述
		var description string
		var version string
		if binary, err := thirdparty_bin.GetBuiltinBinaryByName(name); err == nil {
			description = binary.Description
			version = binary.Version
		}

		// 显示工具名称和状态
		fmt.Printf("   %d. %s", i+1, name)
		if status.Installed {
			fmt.Printf(" [INSTALLED]")
			if status.NeedsUpdate {
				fmt.Printf(" (Update available)")
			}
		} else {
			fmt.Printf(" [NOT INSTALLED]")
		}

		// 显示版本信息
		if version != "" {
			fmt.Printf(" - %s", version)
		}
		fmt.Println()

		// 显示描述
		if description != "" {
			fmt.Printf("      %s\n", description)
		}

		// 显示安装信息
		if status.Installed {
			fmt.Printf("      Install path: %s", status.InstallPath)
			if status.InstalledVersion != "" && status.InstalledVersion != version {
				fmt.Printf(" (version: %s)", status.InstalledVersion)
			}
			fmt.Println()

			if status.AvailableVersion != "" && status.NeedsUpdate {
				fmt.Printf("      Update to %s available\n", status.AvailableVersion)
			}
		}
		fmt.Println()
	}

	return nil
}

func realMain() {
	app := cli.NewApp()
	app.Name = "thirdparty-bin-manager"
	app.Usage = "Yaklang Third-party Binary Tool Manager: Install, uninstall and manage third-party tools"
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
			Usage:       "Enable verbose output",
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
			Usage: "List all registered tools with installation status and descriptions",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "details, d",
					Usage: "Show detailed builtin configuration (platform support, URLs, etc.)",
				},
			},
			Action: func(c *cli.Context) error {
				showDetails := c.Bool("details")

				if showDetails {
					fmt.Println("=== Builtin Binary Tools (Detailed Configuration) ===")
					if err := thirdparty_bin.PrintBuiltinBinaries(); err != nil {
						log.Errorf("Failed to get builtin tools: %v", err)
						return err
					}
					return nil
				}

				// 默认显示所有注册工具的状态和描述
				return showAllRegisteredTools()
			},
		},
		{
			Name:      "install",
			Usage:     "Install specified binary tool",
			ArgsUsage: "<tool-name>",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:        "force, f",
					Usage:       "Force reinstall (overwrite existing files)",
					Destination: &force,
				},
				cli.StringFlag{
					Name:        "proxy, p",
					Usage:       "Use proxy for download (http://proxy:port)",
					Destination: &proxy,
				},
				cli.IntFlag{
					Name:        "timeout, t",
					Usage:       "Download timeout in seconds",
					Value:       300,
					Destination: &timeout,
				},
				cli.StringFlag{
					Name:        "install-dir, d",
					Usage:       "Custom installation directory",
					Destination: &installDir,
				},
			},
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("please specify the tool name to install")
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
				var lastProgress float64 = -1
				var progressStarted bool = false
				options.Progress = func(progress float64, downloaded, total int64, message string) {
					// 特殊处理开始下载消息
					if message == "开始下载" {
						fmt.Printf("Package size: %s\n", utils.ByteSize(uint64(total)))
						fmt.Printf("Starting download...\n")
						progressStarted = true
						return
					}

					// 只在进度有明显变化时更新显示（避免闪烁）
					if progress-lastProgress >= 0.01 || progress >= 1.0 || message != "下载中" {
						// 清除之前的进度条输出
						if progressStarted {
							clearProgressLine()
						}

						progressLine := renderProgressBar(progress, downloaded, total, message)
						fmt.Printf("\r%s", progressLine)
						lastProgress = progress

						// 下载完成时换行
						if progress >= 1.0 {
							fmt.Println()
							progressStarted = false
						}
					}
				}

				fmt.Printf("Installing %s...\n", toolName)

				err := thirdparty_bin.Install(toolName, options)
				if err != nil {
					clearProgressLine()
					fmt.Printf("Installation failed: %v\n", err)
					return err
				}

				clearProgressLine()
				fmt.Printf("%s installed successfully!\n", toolName)

				// 显示安装状态
				status, err := thirdparty_bin.GetStatus(toolName)
				if err == nil {
					fmt.Printf("Install path: %s\n", status.InstallPath)
					if status.InstalledVersion != "" {
						fmt.Printf("Version: %s\n", status.InstalledVersion)
					}
				}

				return nil
			},
		},
		{
			Name:      "uninstall",
			Usage:     "Uninstall specified binary tool",
			ArgsUsage: "<tool-name>",
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("please specify the tool name to uninstall")
				}

				toolName := c.Args().First()

				fmt.Printf("Uninstalling %s...\n", toolName)

				err := thirdparty_bin.Uninstall(toolName)
				if err != nil {
					fmt.Printf("Uninstallation failed: %v\n", err)
					return err
				}

				fmt.Printf("%s uninstalled successfully!\n", toolName)
				return nil
			},
		},
		{
			Name:      "status",
			Usage:     "Show status of specified tool",
			ArgsUsage: "<tool-name>",
			Action: func(c *cli.Context) error {
				if c.NArg() < 1 {
					return fmt.Errorf("please specify the tool name to check status")
				}

				toolName := c.Args().First()

				status, err := thirdparty_bin.GetStatus(toolName)
				if err != nil {
					fmt.Printf("Failed to get status: %v\n", err)
					return err
				}

				fmt.Printf("=== %s Status ===\n", toolName)
				fmt.Printf("Name: %s\n", status.Name)
				fmt.Printf("Installed: %v\n", status.Installed)
				if status.Installed {
					fmt.Printf("Install path: %s\n", status.InstallPath)
					if status.InstalledVersion != "" {
						fmt.Printf("Installed version: %s\n", status.InstalledVersion)
					}
				}
				if status.AvailableVersion != "" {
					fmt.Printf("Available version: %s\n", status.AvailableVersion)
				}
				if status.NeedsUpdate {
					fmt.Printf("Needs update: Yes\n")
				}

				return nil
			},
		},
		{
			Name:  "info",
			Usage: "Show system and package information",
			Action: func(c *cli.Context) error {
				// 显示包信息
				info := thirdparty_bin.GetPackageInfo()
				fmt.Println("=== Package Information ===")
				for key, value := range info {
					fmt.Printf("%s: %v\n", strings.ReplaceAll(key, "_", " "), value)
				}

				// 显示当前系统信息
				sysInfo := thirdparty_bin.GetCurrentSystemInfo()
				fmt.Printf("\n=== Current System ===\n")
				fmt.Printf("Operating System: %s\n", sysInfo.OS)
				fmt.Printf("Architecture: %s\n", sysInfo.Arch)
				fmt.Printf("Platform Key: %s\n", sysInfo.GetPlatformKey())

				return nil
			},
		},
		{
			Name:  "update",
			Usage: "Update all installed tools",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "proxy, p",
					Usage:       "Use proxy for download (http://proxy:port)",
					Destination: &proxy,
				},
				cli.IntFlag{
					Name:        "timeout, t",
					Usage:       "Download timeout in seconds",
					Value:       300,
					Destination: &timeout,
				},
			},
			Action: func(c *cli.Context) error {
				binaries := thirdparty_bin.ListRegisteredNames()
				if len(binaries) == 0 {
					fmt.Println("No registered tools to update")
					return nil
				}

				fmt.Printf("Checking updates for %d tools...\n", len(binaries))

				updated := 0
				for _, name := range binaries {
					status, err := thirdparty_bin.GetStatus(name)
					if err != nil {
						log.Warnf("Failed to get status for %s: %v", name, err)
						continue
					}

					if !status.Installed {
						continue
					}

					if status.NeedsUpdate {
						fmt.Printf("Updating %s...\n", name)

						options := &thirdparty_bin.InstallOptions{
							Force: true, // Force update
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
								fmt.Printf("\r  Progress: %.1f%% (%s/%s)",
									progress*100,
									utils.ByteSize(uint64(downloaded)),
									utils.ByteSize(uint64(total)))
							}
						}

						err = thirdparty_bin.Install(name, options)
						if err != nil {
							fmt.Printf("\n  Failed to update %s: %v\n", name, err)
						} else {
							fmt.Printf("\n  ✓ %s updated successfully\n", name)
							updated++
						}
					}
				}

				if updated == 0 {
					fmt.Println("All tools are up to date")
				} else {
					fmt.Printf("Successfully updated %d tools\n", updated)
				}

				return nil
			},
		},
		{
			Name:  "reinit",
			Usage: "Reinitialize builtin binary tool registry",
			Action: func(c *cli.Context) error {
				fmt.Println("Reinitializing builtin binary tools...")

				err := thirdparty_bin.ReinitializeBuiltinBinaries()
				if err != nil {
					fmt.Printf("Reinitialization failed: %v\n", err)
					return err
				}

				fmt.Println("Reinitialization successful")

				// Show registered tools
				binaries := thirdparty_bin.ListRegisteredNames()
				fmt.Printf("Registered %d tools: %s\n", len(binaries), strings.Join(binaries, ", "))

				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("Program execution failed: %v", err)
		os.Exit(1)
	}
}

func main() {
	realMain()
}
