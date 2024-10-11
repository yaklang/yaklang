package cvequeryops

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

const (
	// https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-modified.json.gz
	LatestCveModifiedDataFeed = "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-modified.json.gz"
	// https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-recent.json.gz
	LatestCveRecentDataFeed = "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-recent.json.gz"
)

var CveDataFeed = map[string]string{
	"CVE-2002.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2002.json.gz",
	"CVE-2003.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2003.json.gz",
	"CVE-2004.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2004.json.gz",
	"CVE-2005.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2005.json.gz",
	"CVE-2006.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2006.json.gz",
	"CVE-2007.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2007.json.gz",
	"CVE-2008.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2008.json.gz",
	"CVE-2009.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2009.json.gz",
	"CVE-2010.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2010.json.gz",
	"CVE-2011.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2011.json.gz",
	"CVE-2012.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2012.json.gz",
	"CVE-2013.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2013.json.gz",
	"CVE-2014.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2014.json.gz",
	"CVE-2015.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2015.json.gz",
	"CVE-2016.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2016.json.gz",
	"CVE-2017.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2017.json.gz",
	"CVE-2018.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2018.json.gz",
	"CVE-2019.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2019.json.gz",
	"CVE-2020.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2020.json.gz",
	"CVE-2021.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2021.json.gz",
	"CVE-2022.json": "https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-2022.json.gz",
}

func init() {
	for i := 2001; i < time.Now().Year()+1; i++ {
		CveDataFeed[fmt.Sprintf("CVE-%d.json", i)] = fmt.Sprintf("https://nvd.nist.gov/feeds/json/cve/1.1/nvdcve-1.1-%d.json.gz", i)
	}
}

// LoadCVE 从本地的CVE json数据加载构造数据库
func LoadCVE(fileDir, DbPath string, years ...int) {
	manager := cveresources.GetManager(DbPath)

	allowed := funk.Map(years, func(i int) string {
		return fmt.Sprintf("CVE-%d.json", i)
	}).([]string)

	var count int
	total := len(CveDataFeed)
	for fileName := range CveDataFeed {
		count++

		if len(years) > 0 && !utils.StringArrayContains(allowed, fileName) {
			continue
		}

		fileName = path.Join(fileDir, fileName)
		startTime := time.Now()
		log.Infof("LoadCVE begin: " + fileName)
		exitNow, err := LoadCVEByFileName(fileName, manager)
		if err != nil {
			log.Errorf("LoadCVE: %v failed: %v", fileName, err)
		}
		endTime := time.Now()
		log.Infof("handle %v cost %v (%v/%v)", fileName, endTime.Sub(startTime).String(), count, total)
		if exitNow {
			break
		}
	}
}

func LoadCVEByFileName(fileName string, manager *cveresources.SqliteManager) (shouldExit bool, err error) {
	CVEContext, err := ioutil.ReadFile(fileName)
	if err != nil {
		return false, err
	}

	var cveFile cveresources.CVEYearFile
	err = json.Unmarshal(CVEContext, &cveFile)
	if err != nil {
		var tail string
		if len(CVEContext) > 20 {
			tail = string(CVEContext[len(CVEContext)-20:])
		} else {
			tail = string(CVEContext)
		}
		err = errors.Errorf("tail of [%v] context: %#v, with err: %v", fileName, tail, err)
		os.Remove(fileName)
		return false, err

	}

	for _, record := range cveFile.CVERecords {
		manager.SaveCVERecord(&record)
	}

	return false, nil
}

// DownLoad 从NVD下载CVE json数据到本地
func DownLoad(dir string, cached bool) error {
	for name, url := range CveDataFeed {
		fileName := filepath.Join(dir, name)
		if cached {
			if utils.GetFirstExistedFile(fileName) != "" {
				log.Infof("skip %v", fileName)
				continue
			}
		}
		log.Infof("start to download from: %v", url)
		resp, _, err := poc.DoGET(url, poc.WithRetryTimes(3))
		if err != nil {
			log.Error(err)
			continue
		}

		log.Infof("start to un-gzip from: %v", url)
		rawData, err := gzip.NewReader(bytes.NewReader(resp.GetBody()))
		if err != nil {
			log.Error(err)
			continue
		}
		log.Infof("start to save to local file: %v", fileName)
		dstFile, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0o666)
		if err != nil {
			return utils.Errorf("open %v failed; %v", fileName, err)
		}
		_, err = io.Copy(dstFile, rawData)
		if err != nil {
			log.Error(err)
			continue
		}
		log.Infof("handle %v finished", dstFile.Name())
	}
	return nil
}
