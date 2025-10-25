package cvequeryops

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

const (
	LatestCveModifiedDataFeed = "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-modified.json.gz"
	LatestCveRecentDataFeed   = "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-recent.json.gz"
)

var CveDataFeed = map[string]string{
	"CVE-2002.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2002.json.gz",
	"CVE-2003.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2003.json.gz",
	"CVE-2004.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2004.json.gz",
	"CVE-2005.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2005.json.gz",
	"CVE-2006.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2006.json.gz",
	"CVE-2007.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2007.json.gz",
	"CVE-2008.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2008.json.gz",
	"CVE-2009.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2009.json.gz",
	"CVE-2010.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2010.json.gz",
	"CVE-2011.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2011.json.gz",
	"CVE-2012.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2012.json.gz",
	"CVE-2013.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2013.json.gz",
	"CVE-2014.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2014.json.gz",
	"CVE-2015.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2015.json.gz",
	"CVE-2016.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2016.json.gz",
	"CVE-2017.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2017.json.gz",
	"CVE-2018.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2018.json.gz",
	"CVE-2019.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2019.json.gz",
	"CVE-2020.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2020.json.gz",
	"CVE-2021.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2021.json.gz",
	"CVE-2022.json": "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-2022.json.gz",
}

func init() {
	for i := 2002; i < time.Now().Year()+1; i++ {
		CveDataFeed[fmt.Sprintf("CVE-%d.json", i)] = fmt.Sprintf("https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-%d.json.gz", i)
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

	// 解析CVE 2.0格式
	var cveFileV2 cveresources.CVEYearFileV2
	err = json.Unmarshal(CVEContext, &cveFileV2)
	if err != nil {
		var tail string
		if len(CVEContext) > 20 {
			tail = string(CVEContext[len(CVEContext)-20:])
		} else {
			tail = string(CVEContext)
		}
		err = errors.Errorf("解析CVE 2.0格式失败 [%v] context: %#v, with err: %v", fileName, tail, err)
		os.Remove(fileName)
		return false, err
	}

	// 处理CVE 2.0格式
	for _, vuln := range cveFileV2.Vulnerabilities {
		manager.SaveCVEVulnerability(&vuln)
	}
	log.Infof("成功加载CVE 2.0格式文件: %v, 记录数: %d", fileName, len(cveFileV2.Vulnerabilities))

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

		// 使用流式处理，避免将大文件读入内存
		var downloadErr error
		_, _, err := poc.DoGET(url,
			poc.WithRetryTimes(3),
			poc.WithSave(false),        // 禁用 HTTP 流保存到数据库
			poc.WithNoBodyBuffer(true), // 禁用响应体缓冲
			poc.WithBodyStreamReaderHandler(func(header []byte, bodyReader io.ReadCloser) {
				defer bodyReader.Close()

				log.Infof("start to un-gzip from: %v", url)
				rawData, err := gzip.NewReader(bodyReader)
				if err != nil {
					downloadErr = utils.Errorf("gzip decompress failed: %v", err)
					log.Error(downloadErr)
					return
				}
				defer rawData.Close()

				log.Infof("start to save to local file: %v", fileName)
				dstFile, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
				if err != nil {
					downloadErr = utils.Errorf("open %v failed: %v", fileName, err)
					log.Error(downloadErr)
					return
				}
				defer dstFile.Close()

				_, err = io.Copy(dstFile, rawData)
				if err != nil {
					downloadErr = utils.Errorf("copy data failed: %v", err)
					log.Error(downloadErr)
					return
				}

				log.Infof("handle %v finished", dstFile.Name())
			}))

		if err != nil {
			log.Errorf("download %v failed: %v", url, err)
			continue
		}

		if downloadErr != nil {
			log.Errorf("process %v failed: %v", url, downloadErr)
			// 清理可能产生的不完整文件
			os.Remove(fileName)
			continue
		}
	}
	return nil
}
