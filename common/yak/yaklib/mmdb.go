package yaklib

import (
	"fmt"
	"github.com/oschwald/maxminddb-golang"
	"io"
	"net"
	"net/http"
	"os"
	"yaklang/common/consts"
	"yaklang/common/geo"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/ziputil"
	"path"
	"path/filepath"
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

var MmdbExports = map[string]interface{}{
	"Open": maxminddb.Open,
	"QueryIPCity": func(r *maxminddb.Reader, ip string) (*geo.City, error) {
		var c geo.City
		err := r.Lookup(net.ParseIP(utils.FixForParseIP(ip)), &c)
		if err != nil {
			return nil, utils.Errorf("loop up failed: %s", err)
		}

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
	},
}

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
