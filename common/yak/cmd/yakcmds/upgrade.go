package yakcmds

import (
	"context"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var UpgradeCommand = cli.Command{
	Name:  "upgrade",
	Usage: "upgrade / reinstall newest or user-defined yak.",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "timeout",
			Usage: "Set Timeout for download yak binary, default 60s.",
			Value: 60,
		},
		cli.StringFlag{
			Name:  "version,v",
			Usage: "Set the version of yak to download, default latest.",
		},

		cli.BoolFlag{
			Name:  "list,l",
			Usage: "Show all active versions.",
		},

		cli.IntFlag{
			Name:  "n",
			Usage: "Show latest N active versions.",
			Value: 16,
		},
	},
	Action: func(c *cli.Context) error {
		const activeVersions = `https://aliyun-oss.yaklang.com/yak/version-info/active_versions.txt`
		if c.Bool("list") {
			rsp, _, err := poc.DoGET(activeVersions)
			if err != nil {
				log.Errorf("fetch active versions failed: %v", err)
				return err
			}
			versions := utils.PrettifyListFromStringSplitEx(string(rsp.GetBody()), "\n")
			if len(versions) == 0 {
				log.Errorf("fetch active versions failed: %v", err)
				return err
			}
			log.Infof("active versions: len: %v", len(versions))
			if c.Int("n") > 0 {
				log.Infof("show latest %v active versions", c.Int("n"))
				versions = versions[:c.Int("n")]
			}
			for _, ver := range versions {
				fmt.Println(ver)
			}
			return nil
		}

		exePath, err := os.Executable()
		exeDir := filepath.Dir(exePath)
		if err != nil {
			return utils.Errorf("cannot fetch os.Executable()...: %s", err)
		}

		version := c.String("version")
		if version == "" {
			rsp, _, err := poc.DoGET(`https://aliyun-oss.yaklang.com/yak/latest/version.txt`, poc.WithTimeout(10))
			if err != nil {
				log.Warnf("fetch latest yak version failed: %v", err)
				return err
			}
			raw := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
			version = strings.TrimSpace(string(raw))
		}

		if version == "" {
			log.Warnf("fetch latest yak version failed: %v use latest", err)
			version = "latest"
		}

		fetchUrl := func(ver string) string {
			return fmt.Sprintf(`https://aliyun-oss.yaklang.com/yak/%v/yak_%v_%v`, ver, runtime.GOOS, runtime.GOARCH)
		}

		downloadUrl := fetchUrl(version)
		rsp, _, err := poc.DoHEAD(downloadUrl, poc.WithTimeout(10))
		if err != nil {
			log.Errorf("fetch yak binary failed: %v", err)
			return err
		}
		_ = rsp
		if rsp.GetStatusCode() >= 400 {
			log.Infof("fetch yak binary failed: %v", rsp.GetStatusCode())
			rsp, _, err := poc.DoGET(activeVersions)
			if err != nil {
				log.Errorf("fetch active versions failed: %v", err)
				return err
			}
			versions := utils.PrettifyListFromStringSplitEx(string(rsp.GetBody()), "\n")
			if len(versions) == 0 {
				log.Errorf("fetch active versions failed: %v", err)
				return err
			}
			log.Infof("active versions: len: %v", len(versions))
			for _, ver := range versions {
				fmt.Println(ver)
			}
			return nil
		}

		newFilePath := filepath.Join(exeDir, "yak.new")
		fd, err := os.OpenFile(newFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o766)
		if err != nil {
			log.Errorf("create temp file failed: %v", err)
			return err
		}
		defer func() {
			fd.Sync()
			fd.Close()
		}()

		binary := downloadUrl
		log.Infof("start to download yak: %v", binary)
		timeout := c.Int("timeout")
		ctx, cancel := context.WithCancel(context.Background())
		_, _, err = poc.DoGET(binary, poc.WithConnectTimeout(float64(timeout)), poc.WithBodyStreamReaderHandler(func(header []byte, bodyReader io.ReadCloser) {
			defer func() {
				cancel()
			}()

			log.Infof("downloading yak binary...")
			contentLength := lowhttp.GetHTTPPacketHeader(header, "content-length")
			writer := progresswriter.New(uint64(codec.Atoi(contentLength)))
			writer.ShowProgress("downloading", ctx)
			_, err := io.Copy(io.MultiWriter(fd, writer), bodyReader)
			if err != nil && err != io.EOF {
				log.Errorf("download yak failed: %v", err)
				return
			}
		}))
		if err != nil {
			log.Errorf("download yak failed: %v", err)
			return err
		}

		destDir, _ := filepath.Split(exePath)
		backupPath := filepath.Join(destDir, fmt.Sprintf("yak_%s", consts.GetYakVersion()))
		if runtime.GOOS == "windows" {
			backupPath += ".exe"
		}
		log.Infof("backup yak old engine to %s", backupPath)

		log.Infof("origin binary: %s", exePath)

		// 备份旧的
		if err := os.Rename(exePath, backupPath); err != nil {
			return utils.Errorf("backup old yak-engine failed: %s, retry re-Install with \n"+
				"    `bash <(curl -sS -L http://oss.yaklang.io/install-latest-yak.sh)`\n\n", err)
		}
		// 覆盖
		if err := os.Rename(newFilePath, exePath); err != nil {
			// rollback
			rerr := os.Rename(backupPath, exePath)
			if rerr != nil {
				return utils.Errorf("rename new yak-engine failed: %s, rollback failed: %s, retry re-Install with \n"+"    `bash <(curl -sS -L http://oss.yaklang.io/install-latest-yak.sh)`\n\n", err, rerr)
			}

			return utils.Errorf("rename new yak-engine failed: %s, retry re-Install with \n"+
				"    `bash <(curl -sS -L http://oss.yaklang.io/install-latest-yak.sh)`\n\n", err)
		}
		return nil
	},
}
