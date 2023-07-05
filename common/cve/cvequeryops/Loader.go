package cvequeryops

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
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
	var total = len(CveDataFeed)
	for fileName := range CveDataFeed {
		count++

		if len(years) > 0 && !utils.StringArrayContains(allowed, fileName) {
			continue
		}

		fileName = path.Join(fileDir, fileName)
		var startTime = time.Now()
		log.Infof("LoadCVE begin: " + fileName)
		_, err := LoadCVEByFileName(fileName, manager)
		var endTime = time.Now()
		log.Infof("handle %v cost %v (%v/%v)", fileName, endTime.Sub(startTime).String(), count, total)
		if err != nil {
			log.Errorf("handle %v failed: %v", fileName, err)
			continue
		}
	}
}

func LoadCVEByFileName(fileName string, manager *cveresources.SqliteManager) (shouldExit bool, err error) {
	CVEContext, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Error(err)
	}

	var cveFile cveresources.CVEYearFile
	err = json.Unmarshal(CVEContext, &cveFile)
	if err != nil {
		log.Error(err)
	}

	for _, record := range cveFile.CVERecords {
		manager.SaveCVERecord(&record)
	}

	return true, nil
}

// DownLoad 从NVD下载CVE json数据到本地
func DownLoad(dir string) error {
	for name, url := range CveDataFeed {
		log.Infof("start to download from: %v", url)
		resp, err := http.Get(url)
		if err != nil {
			log.Error(err)
			continue
		}

		log.Infof("start to un-gzip from: %v", url)
		rawData, err := gzip.NewReader(resp.Body)
		if err != nil {
			log.Error(err)
			continue
		}
		f := filepath.Join(dir, name)
		log.Infof("start to save to local file: %v", f)
		dstFile, err := os.OpenFile(f, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return utils.Errorf("open %v failed; %v", f, err)
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
