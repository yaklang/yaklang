package yaklib

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/oschwald/maxminddb-golang"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/geo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

/*
	(map[string]interface {}) (len=4) {
	 (string) (len=9) "GeoNameID": (float64) 1.814991e+06,
	 (string) (len=17) "IsInEuropeanUnion": (bool) false,
	 (string) (len=7) "IsoCode": (string) (len=2) "CN",
	 (string) (len=5) "Names": (map[string]interface {}) (len=8) {
	  (string) (len=5) "pt-BR": (string) (len=5) "China",
	  (string) (len=2) "ru": (string) (len=10) "Китай",
	  (string) (len=5) "zh-CN": (string) (len=6) "中国",
	  (string) (len=2) "de": (string) (len=5) "China",
	  (string) (len=2) "en": (string) (len=5) "China",
	  (string) (len=2) "es": (string) (len=5) "China",
	  (string) (len=2) "fr": (string) (len=5) "Chine",
	  (string) (len=2) "ja": (string) (len=6) "中国"
	 }
	}

var CNCountry =
*/
var CNCountry = struct {
	GeoNameID         uint              `maxminddb:"geoname_id"`
	IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
	IsoCode           string            `maxminddb:"iso_code"`
	Names             map[string]string `maxminddb:"names"`
}(struct {
	GeoNameID         uint
	IsInEuropeanUnion bool
	IsoCode           string
	Names             map[string]string
}{GeoNameID: 1814991, IsInEuropeanUnion: false, IsoCode: "CN", Names: map[string]string{
	//	(string) (len=5) "pt-BR": (string) (len=5) "China",
	//(string) (len=2) "ru": (string) (len=10) "Китай",
	//(string) (len=5) "zh-CN": (string) (len=6) "中国",
	//(string) (len=2) "de": (string) (len=5) "China",
	//(string) (len=2) "en": (string) (len=5) "China",
	//(string) (len=2) "es": (string) (len=5) "China",
	//(string) (len=2) "fr": (string) (len=5) "Chine",
	//(string) (len=2) "ja": (string) (len=6) "中国"
	"ru":    "Китай",
	"zh-CN": "中国",
	"pt-BR": "China",
	"en":    "China",
	"es":    "China",
	"de":    "China",
	"fr":    "China",
	"ja":    "中国",
}})

var (
	homeDir       = utils.GetHomeDirDefault(".")
	yakitHomeDir  = consts.GetDefaultYakitBaseDir()
	mmdbReader    *maxminddb.Reader
	mmdbISPReader *maxminddb.Reader
	mmdbLocation  = []string{
		path.Join(yakitHomeDir, "GeoIP2-City.mmdb"),
		path.Join(yakitHomeDir, "geoip_directory", "GeoIP2-City.mmdb"),
		"GeoIP2-City.mmdb",
		path.Join(homeDir, "GeoIP2-City.mmdb"),
		path.Join(homeDir, "GeoIP2-City.mmdb"),
		path.Join(homeDir, ".palm-desktop", "GeoIP2-City.mmdb"),
	}
	mmdbISPLocation = []string{
		path.Join(yakitHomeDir, "GeoIP2-ISP.mmdb"),
		path.Join(yakitHomeDir, "geoip_directory", "GeoIP2-ISP.mmdb"),
		"GeoIP2-ISP.mmdb",
		path.Join(homeDir, "GeoIP2-ISP.mmdb"),
		path.Join(homeDir, ".palm-desktop", "GeoIP2-ISP.mmdb"),
	}
)

// mmdbOpen 打开一个 MaxMind mmdb 数据库文件并返回可供查询的 Reader（导出名为 mmdb.Open）
// 常配合 mmdb.QueryIPCity 使用，对 IP 做地理位置归属查询
//
// 参数:
//   - file: mmdb 数据库文件路径（如 GeoIP2-City.mmdb）
//
// 返回值:
//   - mmdb 数据库 Reader
//   - 错误信息（文件不存在或格式非法时返回）
//
// Example:
// ```
// // 该示例依赖本地 GeoIP2-City.mmdb 数据文件，仅作用法示意
// reader = mmdb.Open("GeoIP2-City.mmdb")~
// city = mmdb.QueryIPCity(reader, "1.1.1.1")~
// println(city.City.Names["en"])
// ```
func mmdbOpen(file string) (*maxminddb.Reader, error) {
	return maxminddb.Open(file)
}

// mmdbQueryIPCity 使用已打开的 mmdb Reader 查询指定 IP 的城市级地理信息（导出名为 mmdb.QueryIPCity）
// 对港澳台地区做了归一化处理
//
// 参数:
//   - r: 由 mmdb.Open 返回的数据库 Reader
//   - ip: 待查询的 IP 地址字符串
//
// 返回值:
//   - 包含国家/城市/坐标等信息的地理对象
//   - 错误信息（查询失败时返回）
//
// Example:
// ```
// // 该示例依赖本地 GeoIP2-City.mmdb 数据文件，仅作用法示意
// reader = mmdb.Open("GeoIP2-City.mmdb")~
// city = mmdb.QueryIPCity(reader, "1.1.1.1")~
// println(city.Country.IsoCode)
// ```
func mmdbQueryIPCity(r *maxminddb.Reader, ip string) (*geo.City, error) {
	var c geo.City
	err := r.Lookup(net.ParseIP(utils.FixForParseIP(ip)), &c)
	if err != nil {
		return nil, utils.Errorf("loop up failed: %s", err)
	}

	{

		if c.Country.IsoCode == "HK" || c.Country.IsoCode == "MO" {
			c.City.Names = c.Country.Names
			c.City.GeoNameID = c.Country.GeoNameID
			c.Country = CNCountry
		} else if c.Country.IsoCode == "TW" {
			if c.City.GeoNameID > 0 {
				var maps = make(map[string]string)
				for k, v := range c.City.Names {
					if c.Country.Names[k] != "" {
						maps[k] = fmt.Sprintf("%v/%v", c.Country.Names[k], v)
					}
				}
			} else {
				c.City.Names = c.Country.Names
				c.City.GeoNameID = c.Country.GeoNameID
			}

			c.Country = CNCountry
		}
		return &c, nil
	}
}

var MmdbExports = map[string]interface{}{
	"Open":        mmdbOpen,
	"QueryIPCity": mmdbQueryIPCity,
}

// QueryIPCity 查询一个 IP 的地理位置信息（导出名为 db.QueryIPCity）
// 依赖本地 GeoIP 数据库，可先用 db.DownloadGeoIP 下载
//
// 参数:
//   - ip: 要查询的 IP 地址
//
// 返回值:
//   - 地理位置信息对象（City）
//   - 错误信息（数据库缺失或查询失败时返回）
//
// Example:
// ```
// // 需先准备本地 GeoIP 数据库（示意性示例）
// city = db.QueryIPCity("1.1.1.1")~
// println(city.Country.Names["en"])
// ```
func QueryIP(ip string) (*geo.City, error) {
	var err error
	if mmdbReader == nil {
		mmdbReader, err = maxminddb.Open(utils.GetFirstExistedPath(mmdbLocation...))
		if err != nil {
			return nil, err
		}
	}
	var c geo.City
	err = mmdbReader.Lookup(net.ParseIP(utils.FixForParseIP(ip)), &c)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

// QueryIPForIPS 查询一个 IP 的 ISP（运营商）信息（导出名为 db.QueryIPForIPS）
// 依赖本地 GeoIP ISP 数据库，可先用 db.DownloadGeoIP 下载
//
// 参数:
//   - ip: 要查询的 IP 地址
//
// 返回值:
//   - ISP 信息对象
//   - 错误信息（数据库缺失或查询失败时返回）
//
// Example:
// ```
// // 需先准备本地 GeoIP ISP 数据库（示意性示例）
// isp = db.QueryIPForIPS("1.1.1.1")~
// println(isp.ISP)
// ```
func QueryIPForISP(ip string) (*geo.ISP, error) {
	var err error
	if mmdbISPReader == nil {
		mmdbISPReader, err = maxminddb.Open(utils.GetFirstExistedPath(mmdbISPLocation...))
		if err != nil {
			return nil, err
		}
	}

	var isp geo.ISP
	err = mmdbISPReader.Lookup(net.ParseIP(utils.FixForParseIP(ip)), &isp)
	if err != nil {
		return nil, err
	}
	return &isp, nil
}

// DownloadGeoIP 下载 GeoIP 数据库到本地（导出名为 db.DownloadGeoIP）
// 下载后即可使用 db.QueryIPCity / db.QueryIPForIPS 进行离线 IP 归属查询
//
// 返回值:
//   - 错误信息（下载或解压失败时返回）
//
// Example:
// ```
// // 需要网络访问以下载数据库（示意性示例）
// db.DownloadGeoIP()~
// city = db.QueryIPCity("1.1.1.1")~
// println(city.Country.Names["en"])
// ```
func DownloadMMDB() error {
	base := consts.GetDefaultYakitBaseDir()
	geoipZip := filepath.Join(base, "geoip.zip")
	_ = os.RemoveAll(geoipZip)
	fp, err := os.OpenFile(geoipZip, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return utils.Errorf("open file failed: %s", err)
	}
	defer fp.Close()

	log.Info("start to download geoip")
	rsp, err := http.Get("https://www.yaklang.io/cloudflare/rule/geoip.zip")
	if err != nil {
		return utils.Errorf("download request to geoip failed: %s", err)
	}
	_, err = io.Copy(fp, rsp.Body)
	if err != nil {
		return utils.Errorf("download ... failed: %s", err)
	}

	geoipTarget := filepath.Join(base, "geoip_directory")
	_ = os.RemoveAll(geoipTarget)
	err = ziputil.DeCompress(geoipZip, geoipTarget)
	if err != nil {
		return utils.Errorf("unzip geoip failed: %s", err)
	}
	return nil
}
