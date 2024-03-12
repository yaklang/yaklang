package yakcmds

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

var UpgradeCommand = cli.Command{
	Name:  "upgrade",
	Usage: "upgrade / reinstall newest yak.",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "timeout",
			Usage: "连接超时时间",
			Value: 30,
		},
	},
	Action: func(c *cli.Context) error {
		exePath, err := os.Executable()
		exeDir := filepath.Dir(exePath)
		if err != nil {
			return utils.Errorf("cannot fetch os.Executable()...: %s", err)
		}

		binary := fmt.Sprintf(`https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/yak_%v_%v`, runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			binary = fmt.Sprintf(`https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/yak_%v_%v`, runtime.GOOS, "amd64")
		} else if runtime.GOOS == "windows" {
			binary = fmt.Sprintf(`https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/yak_%v_%v.exe`, runtime.GOOS, "amd64")
		}

		versionUrl := `https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/version.txt`
		timeout := float64(c.Int("timeout"))
		rspIns, _, err := poc.DoGET(versionUrl, poc.WithTimeout(timeout))
		if err != nil {
			log.Errorf("获取 yak 引擎最新版本失败：get yak latest version failed: %v", err)
			return err
		}
		if len(rspIns.RawPacket) > 0 {
			raw := lowhttp.GetHTTPPacketBody(rspIns.RawPacket)
			if len(utils.ParseStringToLines(string(raw))) <= 3 {
				log.Infof("当前 yak 核心引擎最新版本为 / current latest yak core engine version：%v", string(raw))
			}
		}

		log.Infof("start to download yak: %v", binary)
		rspIns, _, err = poc.DoGET(binary, poc.WithTimeout(timeout))
		if err != nil {
			log.Errorf("下载 yak 引擎失败：download yak failed: %v", err)
			return err
		}

		// 设置本地缓存
		newFilePath := filepath.Join(exeDir, "yak.new")
		fd, err := os.OpenFile(newFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o766)
		if err != nil {
			log.Errorf("create temp file failed: %v", err)
			return err
		}

		log.Infof("downloading for yak binary to local")
		_, err = io.Copy(fd, rspIns.MultiResponseInstances[0].Body)
		if err != nil && err != io.EOF {
			log.Errorf("download failed...: %v", err)
			return err
		}
		log.Infof("yak 核心引擎下载成功... / yak engine downloaded")
		fd.Sync()
		fd.Close()

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
