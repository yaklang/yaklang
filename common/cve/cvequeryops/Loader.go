package cvequeryops

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	LatestCveModifiedDataFeed = "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-modified.json.gz"
	LatestCveRecentDataFeed   = "https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-recent.json.gz"
)

// nvdDownloadConcurrency 控制同时下载 NVD feed 的并发数。
// NVD 服务器对适度并发 (4) 不会限流，且能将总下载时间减少到串行的 ~1/4。
const nvdDownloadConcurrency = 4

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

// LoadCVE 从本地 CVE json 数据文件加载并构建 CVE 数据库（导出名为 cve.LoadCVE）
// 参数:
//   - fileDir: 存放 CVE json 数据文件的目录
//   - DbPath: 构建出的数据库文件路径
//   - years: 可选的指定年份，缺省时加载全部
//
// Example:
// ```
// // 示意性示例，需要本地 CVE json 数据
// cve.LoadCVE("/tmp/cve-data", "/tmp/cve.db", 2021, 2022)
// ```
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

// LoadCVEByFileName 解析单个 NVD 2.0 JSON 文件并批量写入数据库。
// 使用 CreateInBatches 多行 INSERT 替代逐条 tx.Save，将 N 条 SQL 降为 N/500 条。
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

	// 批量处理：解析所有 CVE（ToCVE 传 nil 跳过逐条 DB 写入），然后用 CreateInBatches 批量入库
	batchCVEs := make([]*cveresources.CVE, 0, len(cveFileV2.Vulnerabilities))
	productSet := make(map[string]struct{})
	var batchProducts []cveresources.ProductsTable

	for _, vuln := range cveFileV2.Vulnerabilities {
		c, err := vuln.ToCVE(nil) // 传 nil：跳过 extractVendorProduct 中的逐条 db.Save
		if err != nil {
			continue
		}
		if c == nil {
			continue
		}
		batchCVEs = append(batchCVEs, c)

		// 收集 ProductsTable（去重）
		for _, vendor := range cveresources.Set(strings.Split(c.Vendor, ",")) {
			if vendor == "" {
				continue
			}
			for _, product := range cveresources.Set(strings.Split(c.Product, ",")) {
				if product == "" {
					continue
				}
				key := vendor + "/" + product
				if _, exists := productSet[key]; !exists {
					productSet[key] = struct{}{}
					batchProducts = append(batchProducts, cveresources.ProductsTable{
						Product: product,
						Vendor:  vendor,
					})
				}
			}
		}
	}
	log.Infof("成功解析CVE 2.0格式文件: %v, 记录数: %d", fileName, len(batchCVEs))

	// 批量写入 CVE 主表：使用 CreateInBatches 多行 INSERT
	// 相比逐条 tx.Save（每条 2 SQL：UPDATE 尝试 + INSERT），CreateInBatches 每 500 条仅 1 条 SQL
	if len(batchCVEs) > 0 {
		if db := manager.DB.CreateInBatches(batchCVEs, 500); db.Error != nil {
			log.Errorf("CreateInBatches CVE failed: %s", db.Error)
		}
	}

	// 批量写入 ProductsTable（使用 OnConflictDoNothing 避免主键冲突）
	if len(batchProducts) > 0 {
		for i := 0; i < len(batchProducts); i += 500 {
			endIdx := i + 500
			if endIdx > len(batchProducts) {
				endIdx = len(batchProducts)
			}
			manager.DB.OnConflictDoNothing("product").CreateInBatches(batchProducts[i:endIdx], 500)
		}
	}

	return false, nil
}


// DownLoad 从 NVD 下载 CVE json 数据到本地目录（导出名为 cve.Download）
// 参数:
//   - dir: 下载数据保存目录
//   - cached: 为 true 时跳过已存在的文件
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需要网络访问 NVD
// err = cve.Download("/tmp/cve-data", true)
// if err != nil { die(err) }
// ```
// downloadNVDFeed 使用标准 HTTP 库下载 NVD feed 并解压保存
// 比 poc.DoGET 流式处理更稳定，避免 unexpected EOF
func downloadNVDFeed(url, destFile string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return utils.Errorf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return utils.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// 创建 gzip reader
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return utils.Errorf("gzip decompress failed: %v", err)
	}
	defer gzReader.Close()

	// 写入文件
	dst, err := os.OpenFile(destFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		return utils.Errorf("open %s failed: %v", destFile, err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, gzReader)
	if err != nil {
		return utils.Errorf("copy data failed: %v", err)
	}

	log.Infof("downloaded %s -> %s", url, destFile)
	return nil
}

// downloadWithRetry 带重试的单文件下载，NVD 经常因网络问题中断
func downloadWithRetry(url, destFile string) error {
	maxRetries := 5
	var lastErr error
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			log.Infof("retry %d/%d for %v after 5s", retry, maxRetries, url)
			time.Sleep(5 * time.Second)
		}
		log.Infof("start to download from: %v (attempt %d)", url, retry+1)

		err := downloadNVDFeed(url, destFile)
		if err != nil {
			lastErr = err
			log.Errorf("download %v failed (attempt %d): %v", url, retry+1, err)
			os.Remove(destFile)
			continue
		}

		// 下载成功
		return nil
	}
	return lastErr
}

// probeContentLength 发送 HEAD 请求获取文件大小，用于下载排序。
// 失败时返回 0，不影响下载流程。
func probeContentLength(url string) int64 {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Head(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.ContentLength
}

func DownLoad(dir string, cached bool, years ...int) error {
	// 如果指定了 years，只下载对应年份的数据
	allowed := funk.Map(years, func(i int) string {
		return fmt.Sprintf("CVE-%d.json", i)
	}).([]string)

	// 收集需要下载的任务
	type downloadTask struct {
		name   string
		url    string
		size   int64 // Content-Length (gzip), 0 if unknown
	}
	var tasks []downloadTask
	for name, url := range CveDataFeed {
		if len(years) > 0 && !utils.StringArrayContains(allowed, name) {
			log.Infof("skip %v (filtered by year)", name)
			continue
		}
		fileName := filepath.Join(dir, name)
		if cached {
			if utils.GetFirstExistedFile(fileName) != "" {
				log.Infof("skip %v", fileName)
				continue
			}
		}
		tasks = append(tasks, downloadTask{name: name, url: url})
	}

	if len(tasks) == 0 {
		return nil
	}

	// 快速探测各文件大小，用于排序。HEAD 请求很快（~100ms），并发执行不影响总时间。
	var probeWg sync.WaitGroup
	for i := range tasks {
		probeWg.Add(1)
		go func(idx int) {
			defer probeWg.Done()
			tasks[idx].size = probeContentLength(tasks[idx].url)
		}(i)
	}
	probeWg.Wait()

	// 按 Content-Length 降序排序：大文件先下载，小文件填空隙，避免末尾批次全是大文件造成慢尾巴
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].size > tasks[j].size
	})

	log.Infof("download %d NVD feeds with %d concurrent workers (sorted by size desc)", len(tasks), nvdDownloadConcurrency)

	// 并发下载，使用信号量控制并发数
	sem := make(chan struct{}, nvdDownloadConcurrency)
	var wg sync.WaitGroup
	var failedCount int
	var mu sync.Mutex

	for _, task := range tasks {
		wg.Add(1)
		go func(t downloadTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			destFile := filepath.Join(dir, t.name)
			startTime := time.Now()
			if err := downloadWithRetry(t.url, destFile); err != nil {
				log.Errorf("download %v failed after retries: %v", t.url, err)
				mu.Lock()
				failedCount++
				mu.Unlock()
				return
			}
			log.Infof("handle %v cost %v", t.name, time.Since(startTime))
		}(task)
	}
	wg.Wait()

	if failedCount > 0 {
		log.Errorf("download completed with %d/%d failures", failedCount, len(tasks))
	}
	return nil
}
