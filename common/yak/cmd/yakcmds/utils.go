package yakcmds

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/aiforge"

	"github.com/antchfx/xmlquery"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/twofa"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/utils/xmlfmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/samber/lo"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/cybertunnel"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/dap"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/scannode"
)

var UtilsCommands = []*cli.Command{
	VacuumSQLiteCommand,
	createYakToolMetadataCommand(),
	createBuildInForgeMetadataCommand(),
	{
		Name:  "embed-fs-hash", // 此命令会被CI调用，用于自动生成hash.go文件。如果希望在提交代码时自动更新静态资源的哈希值，需要在.github/workflows/update-embed-fs.yml中添加对应的静态资源路径
		Usage: `Generate Current Embed File System(yak/syntaxflow/aiforge/aitool) Hash`,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name: "type",
			},
			cli.BoolFlag{
				Name: "override",
			},
			cli.BoolFlag{
				Name: "all",
			},
		},
		Action: func(c *cli.Context) error {
			templs := []string{
				"common/consts/hash.go",
				"consts/hash.go",
			}
			template := utils.GetFirstExistedFile(templs...)
			if c.Bool("override") && template == "" {
				return utils.Errorf("template not found, in %v", templs)
			}

			rets := []string{strings.ToLower(c.String("type"))}
			if c.Bool("all") {
				rets = []string{"yak", "syntaxflow", "forge", "aitool"}
			}
			for _, ret := range rets {
				switch ret {
				case "coreplugin", "yak", "yaklang":
					result, err := coreplugin.CorePluginHash()
					if err != nil {
						return err
					}
					fmt.Println(result)
					if c.Bool("override") {
						if consts.ExistedCorePluginEmbedFSHash == result {
							continue
						}
						if matched, _ := regexp_utils.NewYakRegexpUtils("[0-9a-fA-F]+").MatchString(result); !matched {
							return utils.Errorf("invalid hash: %v", result)
						}
						templ, err := os.ReadFile(template)
						if err != nil {
							return err
						}

						re := regexp.MustCompile(`(const ExistedCorePluginEmbedFSHash string = ")([a-zA-Z0-9]+)(")`)
						newContent := re.ReplaceAllString(string(templ), "${1}"+result+"${3}")
						err = os.RemoveAll(template + ".bak")
						if err != nil {
							return err
						}
						err = os.Rename(template, template+".bak")
						if err != nil {
							return err
						}
						err = os.WriteFile(template, []byte(newContent), 0o644)
						if err != nil {
							return err
						}
						continue
					}
				case "sf", "syntaxflow", "sast":
					result, err := sfbuildin.SyntaxFlowRuleHash()
					if err != nil {
						return err
					}
					fmt.Println(result)
					if c.Bool("override") {
						if consts.ExistedSyntaxFlowEmbedFSHash == result {
							continue
						}
						if matched, _ := regexp_utils.NewYakRegexpUtils("[0-9a-fA-F]+").MatchString(result); !matched {
							return utils.Errorf("invalid hash: %v", result)
						}
						templ, err := os.ReadFile(template)
						if err != nil {
							return err
						}

						re := regexp.MustCompile(`(const ExistedSyntaxFlowEmbedFSHash string = ")([a-zA-Z0-9]+)(")`)
						newContent := re.ReplaceAllString(string(templ), "${1}"+result+"${3}")
						err = os.RemoveAll(template + ".bak")
						if err != nil {
							return err
						}
						err = os.Rename(template, template+".bak")
						if err != nil {
							return err
						}
						err = os.WriteFile(template, []byte(newContent), 0o644)
						if err != nil {
							return err
						}
					}
				case "forge":
					result, err := aiforge.BuildInForgeHash()
					if err != nil {
						return err
					}
					fmt.Println(result)
					if c.Bool("override") {
						if consts.ExistedBuildInForgeEmbedFSHash == result {
							continue
						}
						if matched, _ := regexp_utils.NewYakRegexpUtils("[0-9a-fA-F]+").MatchString(result); !matched {
							return utils.Errorf("invalid hash: %v", result)
						}
						templ, err := os.ReadFile(template)
						if err != nil {
							return err
						}

						re := regexp.MustCompile(`(const ExistedBuildInForgeEmbedFSHash string = ")([a-zA-Z0-9]*)(")`)
						newContent := re.ReplaceAllString(string(templ), "${1}"+result+"${3}")
						err = os.RemoveAll(template + ".bak")
						if err != nil {
							return err
						}
						err = os.Rename(template, template+".bak")
						if err != nil {
							return err
						}
						err = os.WriteFile(template, []byte(newContent), 0o644)
						if err != nil {
							return err
						}
					}
				case "aitool":
					result, err := yakscripttools.BuildInAIToolHash()
					if err != nil {
						return err
					}
					fmt.Println(result)
					if c.Bool("override") {
						if consts.ExistedBuildInAIToolEmbedFSHash == result {
							continue
						}
						if matched, _ := regexp_utils.NewYakRegexpUtils("[0-9a-fA-F]+").MatchString(result); !matched {
							return utils.Errorf("invalid hash: %v", result)
						}
						templ, err := os.ReadFile(template)
						if err != nil {
							return err
						}

						re := regexp.MustCompile(`(const ExistedBuildInAIToolEmbedFSHash string = ")([a-zA-Z0-9]*)(")`)
						newContent := re.ReplaceAllString(string(templ), "${1}"+result+"${3}")
						err = os.RemoveAll(template + ".bak")
						if err != nil {
							return err
						}
						err = os.Rename(template, template+".bak")
						if err != nil {
							return err
						}
						err = os.WriteFile(template, []byte(newContent), 0o644)
						if err != nil {
							return err
						}
					}
				default:
					if ret == "" {
						return utils.Error("empty type")
					}
					return utils.Error("invalid type: " + c.String("type"))
				}
				continue
			}
			return nil
		},
	},
	{
		Name: "http-server",
		Flags: []cli.Flag{
			cli.IntFlag{
				Name:  "port",
				Usage: "default-ports default 8089",
				Value: 8089,
			},
			cli.StringFlag{Name: "d,dir", Usage: "which directory do u want to serve"},
			cli.StringFlag{Name: "f,file", Usage: "which file do u want to serve"},
		},
		Action: func(c *cli.Context) error {
			lis, err := net.Listen("tcp", utils.HostPort("0.0.0.0", c.Int("port")))
			if err != nil {
				return err
			}

			dirname := c.String("dir")
			filename := c.String("file")
			if dirname == "" && filename == "" {
				return utils.Errorf("directory or file should be set")
			}
			if dirname != "" {
				log.Infof("input: -d %v", dirname)
			}
			if filename != "" {
				log.Infof("input: -f %v", filename)
			}

			ifaces, err := pcap.FindAllDevs()
			if err != nil {
				return err
			}
			for _, iface := range ifaces {
				for _, addr := range iface.Addresses {
					v4 := addr.IP.String()
					if !utils.IsIPv4(v4) {
						continue
					}
					log.Infof("serve on: http://%v:%v", v4, c.Int("port"))
				}
			}

			if utils.IsDir(dirname) && dirname != "" {
				log.Infof("enable fileserver for dir: %v", dirname)
				err = http.Serve(lis, http.FileServer(http.Dir(dirname)))
				if err != nil {
					return err
				}
			} else {
				log.Infof("enable fileserver for file: %v", filename)
				err = http.Serve(lis, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					http.ServeFile(writer, request, filename)
				}))
				if err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		Name:  "gzip",
		Usage: "gzip data or file",
		Flags: []cli.Flag{
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
				targetFp, err := os.OpenFile(outFile, os.O_CREATE|os.O_RDWR, 0o666)
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
			fp, err := os.OpenFile(gf, os.O_CREATE|os.O_RDWR, 0o666)
			if err != nil {
				return err
			}
			defer fp.Close()
			gzipWriter := gzip.NewWriter(fp)
			io.Copy(gzipWriter, originFp)
			gzipWriter.Flush()
			gzipWriter.Close()
			return nil
		},
	},
	{
		Name: "hex",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "f,file",
				Usage: "input file",
			},
			cli.StringFlag{
				Name:  "d,data",
				Usage: "input data",
			},
		},
		Usage: "hex encode file or data to hex string",
		Action: func(c *cli.Context) {
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
		},
	},
	{
		Name:  "tag-stats",
		Usage: "Generate Tag Status(for Yakit)",
		Action: func(c *cli.Context) error {
			stats, err := yaklib.NewTagStat()
			if err != nil {
				return err
			}
			for _, v := range stats.All() {
				if v.Count <= 1 {
					continue
				}
				fmt.Printf("TAG:[%v]-%v\n", v.Name, v.Count)
			}
			return nil
		},
	},

	// dap
	{
		Name:  "dap",
		Usage: "Start a server based on the Debug Adapter Protocol (DAP) to debug scripts.",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "host", Usage: "debugger adapter listen host"},
			cli.IntFlag{Name: "port", Usage: "debugger adapter listen port"},
			cli.BoolFlag{Name: "debug", Usage: "debug mode"},
			cli.BoolFlag{Name: "version,v", Usage: "show dap version"},
		},
		Action: func(c *cli.Context) error {
			host := c.String("host")
			port := c.Int("port")
			debug := c.Bool("debug")
			versionFlag := c.Bool("version")
			if versionFlag {
				fmt.Printf("Debugger Adapter version: %v\n", dap.DAVersion)
				return nil
			}

			// 设置日志级别
			if debug {
				log.SetLevel(log.DebugLevel)
			}

			server, stopChan, err := dap.StartDAPServer(host, port)
			if err != nil {
				return err
			}
			defer server.Stop()

			forceStop := make(chan struct{})
			select {
			case <-stopChan:
			case <-forceStop:
			}

			return nil
		},
	},

	// fmt
	{
		Name:  "fmt",
		Usage: "Formatter for Yaklang Code",
		Flags: []cli.Flag{
			cli.BoolFlag{Name: "version,v", Usage: "show formatter version"},
		},
		Action: func(c *cli.Context) error {
			if c.Bool("version") {
				fmt.Printf("Formatter version: %v\n", yakast.FormatterVersion)
				return nil
			}
			args := c.Args()
			file := args[0]
			if file != "" {
				var err error
				absFile := file
				if !filepath.IsAbs(absFile) {
					absFile, err = filepath.Abs(absFile)
					if err != nil {
						return utils.Errorf("fetch abs file path failed: %s", err)
					}
				}
				raw, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				vt := yakast.NewYakCompiler()
				vt.Compiler(string(raw))
				fmt.Printf("%s", vt.GetFormattedCode())
			} else {
				return utils.Errorf("empty yak file")
			}
			return nil
		},
	},

	{
		Name:  "fuzz",
		Usage: "fuzztag short for fuzz tag, fuzz tag is a tool to generate fuzz string for fuzz testing",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "t,target",
				Usage: "Fuzztag Template, like: `{{int(1-10)}}`",
			},
		},
		Action: func(c *cli.Context) {
			for _, r := range mutate.MutateQuick(c.String("t")) {
				println(r)
			}
		},
	},
	// sha256
	{
		Name:  "sha256",
		Usage: "(Inner command) sha256 checksums for file and generate [filename].sha256.txt",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "file,f", Usage: "file to checksum"},
		},
		Action: func(c *cli.Context) error {
			filename := c.String("file")
			if filename == "" {
				return utils.Errorf("empty filename")
			}
			file, err := os.Open(filename)
			if err != nil {
				return err
			}
			defer func() {
				file.Close()
			}()
			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err != nil && err != io.EOF {
				return err
			}
			sum := hasher.Sum(nil)
			result := codec.EncodeToHex(sum)

			targetFile := filename + ".sha256.txt"
			err = os.WriteFile(targetFile, []byte(result), 0o644)
			if err != nil {
				return err
			}
			fmt.Printf("file[%s] Sha256 checksum: %s\nGenerate to %s", filename, result, targetFile)
			return nil
		},
	},
	{
		Name:  "repos-tag",
		Usage: "(Inner command) Get Current Git Repository Tag, if not found, generate a fallback tag with dev/{date}",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "output,o", Usage: "output file", Value: "tags.txt"},
		},
		Action: func(c *cli.Context) error {
			var err error
			fallback := func(suffix string) error {
				results := "dev/" + utils.DatePretty() + suffix
				return os.WriteFile(c.String("output"), []byte(results), 0o644)
			}
			rp, err := git.PlainOpen(".")
			if err != nil {
				return fallback("")
			}
			ref, err := rp.Head()
			if err != nil {
				return fallback("")
			}
			var suffix string
			if ref != nil && !ref.Hash().IsZero() {
				h := ref.Hash().String()
				if len(h) > 8 {
					suffix = "-" + h[:8]
				} else {
					suffix = "-" + h
				}
			}
			// 尝试获取当前 HEAD 关联的所有标签
			tags, err := rp.Tags()
			if err != nil {
				return fallback(suffix)
			}

			// 查找与当前 HEAD 提交相关联的标签
			var foundTags []string
			err = tags.ForEach(func(t *plumbing.Reference) error {
				if t.Hash() == ref.Hash() {
					foundTags = append(foundTags, t.Name().Short())
				}
				return nil
			})
			if err != nil {
				return fallback(suffix)
			}

			if len(foundTags) > 0 {
				return os.WriteFile(c.String("output"), []byte(strings.TrimLeft(foundTags[0], "v")), 0o644)
			}
			return fallback(suffix)
		},
	},
	// upload to oss
	{
		Name:  "upload-oss",
		Usage: "(Inner command) Upload File To Aliyun OSS",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "file,f", Usage: "local_file_path:remote_file_path, splited by ;"},
			cli.StringFlag{Name: "ak", Usage: "Aliyun Access Key"},
			cli.StringFlag{Name: "sk", Usage: "Aliyun Secret Key"},
			cli.StringFlag{Name: "endpoint", Usage: "Aliyun OSS Endpoint", Value: `oss-accelerate.aliyuncs.com`},
			cli.StringFlag{Name: "bucket, b", Usage: "Aliyun OSS Bucket", Value: "yaklang"},
			cli.IntFlag{Name: "times,t", Usage: "retry times", Value: 5},
		},
		Action: func(c *cli.Context) error {
			client, err := oss.New(c.String("endpoint"), c.String("ak"), c.String("sk"), oss.EnableCRC(true))
			if err != nil {
				return err
			}

			bucket, err := client.Bucket(c.String("bucket"))
			if err != nil {
				return err
			}
			for _, i := range strings.Split(c.String("file"), ";") {
				localFilePath, remoteFilePath, ok := strings.Cut(i, ":")
				if !ok {
					return utils.Errorf("invalid file path: %v", i)
				}
				localFilePath = strings.TrimSpace(localFilePath)
				remoteFilePath = strings.TrimSpace(strings.TrimLeft(remoteFilePath, "/"))

				_, _, err = lo.AttemptWithDelay(c.Int("times"), time.Second, func(index int, _ time.Duration) error {
					return bucket.PutObjectFromFile(remoteFilePath, localFilePath)
				})
				if err != nil {
					return utils.Wrap(err, "upload file to oss failed")
				}
			}

			return nil
		},
	},
	// file tree size
	{
		Name:  "weight",
		Usage: "weight dir with depth",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "dir,d", Usage: "dir to weight"},
			cli.IntFlag{Name: "depth", Usage: "depth to weight", Value: 1},
			cli.BoolFlag{Name: "asc", Usage: "sort asc"},
			cli.StringFlag{Name: "blacklist,exclude", Usage: "ignore blacklist", Value: "*_test.go|.git*|*testdata*"},
			cli.StringFlag{Name: "show-exclude", Usage: "filter result", Value: "*.md|*.yak|*.DS_Store|*License|*.g4"},
			cli.IntFlag{Name: "show-min-size", Usage: "show min size", Value: 100000},
		},
		Action: func(c *cli.Context) error {
			m := omap.NewOrderedMap(map[string]int64{})
			err := filesys.Recursive(c.String("dir"), filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
				if c.String("blacklist") != "" {
					if utils.MatchAnyOfGlob(pathname, utils.PrettifyListFromStringSplitEx(c.String("blacklist"), "|")...) {
						return nil
					}
				}
				log.Infof("path: %v, size: %v verbose: %v", pathname, info.Size(), utils.ByteSize(uint64(info.Size())))
				m.Set(pathname, info.Size())
				return nil
			}))
			if err != nil {
				return err
			}
			forest, err := utils.GeneratePathTrees(m.Keys()...)
			if err != nil {
				return err
			}

			results := omap.NewOrderedMap(make(map[string]int64))
			forest.Recursive(func(node2 *utils.PathNode) {
				if node2.GetDepth() > c.Int("depth") {
					return
				}
				count := int64(0)
				for _, child := range node2.AllChildren() {
					size, ok := m.Get(child.Path)
					if !ok {
						log.Warnf("path: %v, name: %v not found", child.Path, child.Name)
						continue
					}
					count += size
				}
				results.Set(node2.Path, count)
			})

			var desc []*sizeDescription
			results.ForEach(func(i string, v int64) bool {
				if c.String("show-exclude") != "" {
					if utils.MatchAnyOfGlob(i, utils.PrettifyListFromStringSplitEx(c.String("show-exclude"), "|")...) {
						return true
					}
				}
				desc = append(desc, &sizeDescription{Name: i, Size: uint64(v)})
				return true
			})

			sort.Slice(desc, func(i, j int) bool {
				if c.Bool("asc") {
					return desc[i].Size < desc[j].Size
				}
				return desc[i].Size > desc[j].Size
			})

			for _, i := range desc {
				fmt.Printf("[%6s]: %v\n", utils.ByteSize(i.Size), i.Name)
			}
			return nil
		},
	},

	// totp forward
	{
		Name: "totp-forward",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "secret",
				Usage: "totp secret",
			},
			cli.StringFlag{
				Name:  "proxy-for",
				Usage: "which port for forwarding to",
			},
			cli.IntFlag{
				Name: "listen,l", Usage: "which port for listening", Value: 8084,
			},
		},
		Action: func(c *cli.Context) {
			var secret string = c.String("secret")
			var lisPort = c.Int("listen")
			if lisPort <= 0 {
				lisPort = 8084
			}
			if secret == "" {

			}

			if secret == "" {
				log.Warn("empty secret")
				return
			}

			for {
				err := twofa.NewOTPServer(secret, lisPort, c.String("proxy-for")).Serve()
				if err != nil {
					log.Errorf("failed to serve: %v", err)
					time.Sleep(time.Second)
					continue
				}
			}
		},
	},

	// mcp
	mcp.MCPCommand,

	{
		Name:    "get-random-port",
		Aliases: []string{"random-port"},
		Flags: []cli.Flag{
			cli.StringFlag{Name: "type", Value: "tcp", Usage: "tcp or udp"},
			cli.BoolFlag{Name: "json", Usage: "json output"},
		},
		Usage: "Get Random Available Port",
		Action: func(c *cli.Context) error {
			hash := "f345213fb48cc9370b2abc97429f8e6e98d07fa0bad8577626af6bc8067c1d18" // sha256("get-random-port")
			port := 0
			switch c.String("type") {
			case "tcp":
				port = utils.GetRandomAvailableTCPPort()
			case "udp":
				port = utils.GetRandomAvailableUDPPort()
			default:
				return utils.Errorf("invalid type: [%v] should be tcp/udp ", c.String("type"))
			}
			if port == 0 {
				return utils.Errorf("not get port with: %v", c.String("type"))
			}

			if c.Bool("json") {
				// fmt.Println("please use regexp: ^<f345213fb48cc9370b2abc97429f8e6e98d07fa0bad8577626af6bc8067c1d18>({.*})<\\/f345213fb48cc9370b2abc97429f8e6e98d07fa0bad8577626af6bc8067c1d18>$")
				fmt.Printf(`<%s>{"port":%d}</%s>`, hash, port, hash)
				return nil
			} else {
				fmt.Printf("port: %d", port)
			}
			return nil
		},
	},
	XPathCommand,
}

var DistributionCommands = []*cli.Command{
	&scannode.DistYakCommand,
	{
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
	},
	{
		Name:  "tunnel",
		Usage: "Create Tunnel For CyberTunnel Service",
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
		Name:    "inspect-tuns",
		Usage:   "Inspect Registered Tunnels",
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

var XPathCommand = &cli.Command{
	Name:    "xpath",
	Usage:   "xpath query",
	Aliases: []string{},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "o",
			Usage: "output to file",
		},
	},
	Action: func(c *cli.Context) error {
		// if no --argument, just use os.Arg
		if len(c.Args()) == 0 {
			return utils.Errorf("empty xpath")
		}

		var Query string
		var Targets []string
		var outputFile string

		if c.String("o") != "" {
			outputFile = c.String("o")
			// Create or truncate the output file
			f, err := os.Create(outputFile)
			if err != nil {
				return utils.Errorf("failed to create output file: %s, err: %v", outputFile, err)
			}
			defer f.Close()
		}

		if len(c.Args()) == 1 {
			Query = c.Args()[0]
			Targets = []string{"."} // current path
		}

		if len(c.Args()) > 1 {
			Query = c.Args()[0]
			Targets = c.Args()[1:]
		}

		show := func(node ...*xmlquery.Node) {
			var output strings.Builder
			for index, i := range node {
				output.WriteString(fmt.Sprintf("\n<!--                   [%d]                   -->\n", index))
				if i == nil {
					continue
				}
				xml := i.OutputXML(true)
				if formatted := xmlfmt.FormatXML(xml, "", "  ", true); formatted != "" {
					output.WriteString(fmt.Sprintf("%v", formatted))
				} else {
					output.WriteString(fmt.Sprintf("%v", xml))
				}
				output.WriteString("\n")
			}

			if outputFile != "" {
				// Append to the output file
				f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					log.Errorf("failed to open output file: %s, err: %v", outputFile, err)
					return
				}
				defer f.Close()

				_, err = f.WriteString(output.String())
				if err != nil {
					log.Errorf("failed to write to output file: %s, err: %v", outputFile, err)
				}
			} else {
				// Write to stdout
				fmt.Print(output.String())
			}
		}

		query := func(filename string) error {
			// file
			log.Infof("query from file: %s", filename)
			fp, err := os.Open(filename)
			if err != nil {
				return err
			}
			defer fp.Close()
			doc, err := xmlquery.Parse(fp)
			if err != nil {
				return err
			}
			nodes := xmlquery.Find(doc, Query)
			show(nodes...)
			return nil
		}

		for _, target := range Targets {
			if utils.IsFile(target) {
				err := query(target)
				if err != nil {
					log.Errorf("failed to query dir: %s, err: %v", target, err)
					continue
				}
			}

			if utils.IsDir(target) {
				// is dir load each .xml file
				err := filesys.Recursive(
					target,
					filesys.WithFileSystem(filesys.NewLocalFs()),
					filesys.WithFileStat(func(name string, fi fs.FileInfo) error {
						if !strings.HasSuffix(name, ".xml") {
							return nil
						}
						return query(name)
					}),
				)
				if err != nil {
					log.Errorf("failed to query dir: %s, err: %v", target, err)
					continue
				}
			}

		}
		if outputFile != "" {
			log.Infof("Results written to: %s", outputFile)
		}
		return nil
	},
}

type sizeDescription struct {
	Name string
	Size uint64
}
