package cvequeryops

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// cveawg 单个 CVE API（无需认证）
	CVEAWGSingleAPI = "https://cveawg.mitre.org/api/cve/"
	// cvelistV5 GitHub Releases API
	CVEListV5ReleasesAPI = "https://api.github.com/repos/CVEProject/cvelistV5/releases"
	// 批量处理的并发数
	v5BatchWorkers = 8
	// 批量提交的批次大小
	v5BatchSize = 500
)

// githubRelease 用于解析 GitHub Releases API 响应
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt string        `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// DownloadCVEV5 下载 cvelistV5 全量 zip 并解压到指定目录
// 全量 zip 约 567MB，包含所有已发布的 CVE 5.0 Record
func DownloadCVEV5(dir string) error {
	os.MkdirAll(dir, 0o755)

	// 1. 获取最新 release
	log.Info("start to fetch latest cvelistV5 release")
	release, err := fetchLatestCVEV5Release()
	if err != nil {
		return utils.Errorf("fetch latest release failed: %v", err)
	}
	log.Infof("latest release: %s, published: %s", release.TagName, release.PublishedAt)

	// 2. 找到全量 zip asset
	var allAsset *githubAsset
	for i, asset := range release.Assets {
		if strings.Contains(asset.Name, "all_CVEs_at_midnight") {
			allAsset = &release.Assets[i]
			break
		}
	}
	if allAsset == nil {
		return utils.Errorf("no all_CVEs_at_midnight asset found in release %s", release.TagName)
	}
	log.Infof("found all CVEs zip: %s (%.0f MB)", allAsset.Name, float64(allAsset.Size)/1024/1024)

	// 3. 下载 zip
	zipPath := filepath.Join(dir, allAsset.Name)
	log.Infof("start to download: %s", allAsset.BrowserDownloadURL)
	err = downloadFile(allAsset.BrowserDownloadURL, zipPath)
	if err != nil {
		return utils.Errorf("download failed: %v", err)
	}
	log.Infof("download finished: %s", zipPath)

	// 4. 解压 zip
	extractDir := filepath.Join(dir, "cvelistV5")
	log.Infof("start to extract to: %s", extractDir)
	err = unzipFile(zipPath, extractDir)
	if err != nil {
		return utils.Errorf("unzip failed: %v", err)
	}
	log.Infof("extract finished: %s", extractDir)

	// 5. 删除 zip 文件节省空间
	os.Remove(zipPath)

	// 6. 处理双层 zip：GitHub release 的全量包文件名是 xxx.zip.zip，
	//    解压后可能得到一个内层 zip，需要再解压一次
	innerZips, _ := findInnerZips(extractDir)
	for _, innerZip := range innerZips {
		log.Infof("found inner zip: %s, extracting...", innerZip)
		innerExtractDir := filepath.Dir(innerZip)
		if err := unzipFile(innerZip, innerExtractDir); err != nil {
			log.Warnf("extract inner zip %s failed: %v", innerZip, err)
			continue
		}
		os.Remove(innerZip)
	}

	return nil
}

// findInnerZips 在目录中查找嵌套的 zip 文件
func findInnerZips(dir string) ([]string, error) {
	var zips []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".zip") {
			zips = append(zips, path)
		}
		return nil
	})
	return zips, err
}

// findCvesDir 递归查找包含 cves 目录的路径（处理多层解压的情况）
func findCvesDir(root string) string {
	var found string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && filepath.Base(path) == "cves" {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// DownloadCVEV5Delta 下载最新的 cvelistV5 delta zip 并返回 CVE 5.0 Records
// delta zip 约 100-200KB，包含最近一小时更新的 CVE
func DownloadCVEV5Delta() ([]*cveresources.CVERecordV5, error) {
	log.Info("start to fetch latest cvelistV5 delta release")
	release, err := fetchLatestCVEV5Release()
	if err != nil {
		return nil, utils.Errorf("fetch latest release failed: %v", err)
	}

	// 找到 delta asset
	var deltaAsset *githubAsset
	for i, asset := range release.Assets {
		if strings.Contains(asset.Name, "delta_CVEs") {
			deltaAsset = &release.Assets[i]
			break
		}
	}
	if deltaAsset == nil {
		return nil, utils.Errorf("no delta asset found in release %s", release.TagName)
	}
	log.Infof("found delta zip: %s (%.0f KB)", deltaAsset.Name, float64(deltaAsset.Size)/1024)

	// 下载到临时文件
	tmpFile, err := ioutil.TempFile("", "cve-v5-delta-*.zip")
	if err != nil {
		return nil, utils.Errorf("create temp file failed: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	err = downloadFile(deltaAsset.BrowserDownloadURL, tmpFile.Name())
	if err != nil {
		return nil, utils.Errorf("download delta failed: %v", err)
	}

	// 解压并解析
	records, err := parseDeltaZip(tmpFile.Name())
	if err != nil {
		return nil, utils.Errorf("parse delta zip failed: %v", err)
	}
	log.Infof("parsed %d CVE records from delta", len(records))

	return records, nil
}

// LoadCVEV5FromDir 从解压后的 cvelistV5 目录加载所有 CVE 5.0 Records 并合并到数据库
// 目录结构: cves/YYYY/NNxxx/CVE-YYYY-NNNNN.json
// 使用批量处理模式：先收集所有文件路径，然后并发解析+批量写入
func LoadCVEV5FromDir(dir string, manager *cveresources.SqliteManager, years ...int) error {
	cvesDir := filepath.Join(dir, "cves")
	if utils.GetFirstExistedPath(cvesDir) == "" {
		// 可能解压后有多层目录（如 cvelistV5/cvelistV5/cves/），递归查找 cves 目录
		found := findCvesDir(dir)
		if found != "" {
			cvesDir = found
		} else {
			cvesDir = dir
		}
	}

	// 第一阶段：快速扫描所有 JSON 文件路径
	// 如果指定了 years，只遍历对应年份的子目录（如 cves/2025/）
	log.Infof("start to scan CVE V5 files in: %s", cvesDir)
	startTime := time.Now()
	var filePaths []string

	if len(years) > 0 {
		// 按年份过滤：只遍历 cves/{year}/ 目录
		for _, year := range years {
			yearDir := filepath.Join(cvesDir, fmt.Sprintf("%d", year))
			if utils.GetFirstExistedPath(yearDir) == "" {
				log.Warnf("year directory not found: %s", yearDir)
				continue
			}
			filepath.Walk(yearDir, func(filePath string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() || !strings.HasSuffix(filePath, ".json") {
					return nil
				}
				filePaths = append(filePaths, filePath)
				return nil
			})
		}
	} else {
		err := filepath.Walk(cvesDir, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(filePath, ".json") {
				return nil
			}
			filePaths = append(filePaths, filePath)
			return nil
		})
		if err != nil {
			return utils.Errorf("walk cves dir failed: %v", err)
		}
	}
	log.Infof("found %d CVE V5 files in %v", len(filePaths), time.Since(startTime))

	if len(filePaths) == 0 {
		return nil
	}

	// 第二阶段：并发读取+解析，批量写入数据库
	db := manager.DB
	totalCount := len(filePaths)
	processedCount := 0

	// 使用通道分发文件路径
	fileChan := make(chan string, v5BatchSize*2)
	type v5Result struct {
		cve  *cveresources.CVE
		cveID string
	}
	resultChan := make(chan *v5Result, v5BatchSize*2)

	// 启动 worker 协程并发解析文件
	var wg sync.WaitGroup
	for i := 0; i < v5BatchWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range fileChan {
				data, err := ioutil.ReadFile(filePath)
				if err != nil {
					continue
				}
				var record cveresources.CVERecordV5
				err = json.Unmarshal(data, &record)
				if err != nil {
					continue
				}
				cve, err := record.ToCVE(nil)
				if err != nil {
					continue // REJECTED 等
				}
				resultChan <- &v5Result{cve: cve, cveID: cve.CVE}
			}
		}()
	}

	// 启动 collector 协程：收集结果，批量写入 DB
	done := make(chan error, 1)
	go func() {
		defer func() {
			close(done)
		}()

		batch := make([]*cveresources.CVE, 0, v5BatchSize)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			err := batchMergeCVEV5Fields(db, batch)
			if err != nil {
				log.Warnf("batch merge failed: %v", err)
			}
			processedCount += len(batch)
			if processedCount%5000 < v5BatchSize {
				log.Infof("processed %d/%d CVE V5 records (%.0f%%)",
					processedCount, totalCount,
					float64(processedCount)/float64(totalCount)*100)
			}
			batch = batch[:0]
		}

		for result := range resultChan {
			batch = append(batch, result.cve)
			if len(batch) >= v5BatchSize {
				flush()
			}
		}
		flush() // 处理剩余
	}()

	// 分发文件路径给 worker
	for _, fp := range filePaths {
		fileChan <- fp
	}
	close(fileChan)
	wg.Wait()
	close(resultChan)

	err := <-done
	log.Infof("total loaded %d CVE V5 records in %v", processedCount, time.Since(startTime))
	return err
}

// batchMergeCVEV5Fields 批量合并 5.0 字段，使用事务减少 DB 交互
func batchMergeCVEV5Fields(db *gorm.DB, cves []*cveresources.CVE) error {
	if len(cves) == 0 {
		return nil
	}

	// 收集所有 CVE ID
	cveIDs := make([]string, len(cves))
	for i, c := range cves {
		cveIDs[i] = c.CVE
	}

	// 一次性查询所有已有记录
	var existingList []cveresources.CVE
	if err := db.Where("cve IN ?", cveIDs).Find(&existingList).Error; err != nil {
		// 如果批量查询失败，回退到逐条处理
		for _, cve := range cves {
			cveresources.MergeCVEV5Fields(db, cve)
		}
		return nil
	}

	// 构建 existing map
	existingMap := make(map[string]*cveresources.CVE, len(existingList))
	for i := range existingList {
		existingMap[existingList[i].CVE] = &existingList[i]
	}

	// 使用事务批量更新
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	for _, v5 := range cves {
		existing, found := existingMap[v5.CVE]
		if !found {
			// 记录不存在，创建
			if err := tx.Save(v5).Error; err != nil {
				tx.Rollback()
				return err
			}
			continue
		}

		updates := map[string]interface{}{}
		if existing.Title == "" && v5.Title != "" {
			updates["title"] = v5.Title
		}
		if existing.Solution == "" && v5.Solution != "" {
			updates["solution"] = v5.Solution
		}
		if existing.Vendor == "" && v5.Vendor != "" {
			updates["vendor"] = v5.Vendor
		}
		if existing.Product == "" && v5.Product != "" {
			updates["product"] = v5.Product
		}
		if existing.DescriptionMain == "" && v5.DescriptionMain != "" {
			updates["description_main"] = v5.DescriptionMain
		}
		if existing.CWE == "" && v5.CWE != "" {
			updates["cwe"] = v5.CWE
		}
		if existing.CVSSVersion == "" && v5.CVSSVersion != "" {
			updates["cvss_version"] = v5.CVSSVersion
			updates["cvss_vector_string"] = v5.CVSSVectorString
			updates["access_vector"] = v5.AccessVector
			updates["access_complexity"] = v5.AccessComplexity
			updates["authentication"] = v5.Authentication
			updates["confidentiality_impact"] = v5.ConfidentialityImpact
			updates["integrity_impact"] = v5.IntegrityImpact
			updates["availability_impact"] = v5.AvailabilityImpact
			updates["base_cvs_sv2_score"] = v5.BaseCVSSv2Score
			updates["severity"] = v5.Severity
		}
		if existing.References == nil && v5.References != nil {
			updates["references"] = v5.References
		}
		if existing.PublishedDate.IsZero() && !v5.PublishedDate.IsZero() {
			updates["published_date"] = v5.PublishedDate
		}
		if existing.LastModifiedData.IsZero() && !v5.LastModifiedData.IsZero() {
			updates["last_modified_data"] = v5.LastModifiedData
		}

		if len(updates) == 0 {
			continue
		}

		if err := tx.Model(&cveresources.CVE{}).Where("cve = ?", v5.CVE).Updates(updates).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// FetchCVEV5Record 通过 cveawg API 实时查询单个 CVE 的 5.0 Record
func FetchCVEV5Record(cveID string) (*cveresources.CVERecordV5, error) {
	url := CVEAWGSingleAPI + cveID
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, utils.Errorf("fetch %s failed: %v", cveID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, utils.Errorf("fetch %s failed: status %d", cveID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, utils.Errorf("read body failed: %v", err)
	}

	var record cveresources.CVERecordV5
	err = json.Unmarshal(body, &record)
	if err != nil {
		return nil, utils.Errorf("unmarshal failed: %v", err)
	}
	return &record, nil
}

// fetchLatestCVEV5Release 获取 cvelistV5 最新 release 信息
func fetchLatestCVEV5Release() (*githubRelease, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(CVEListV5ReleasesAPI + "?per_page=1")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var releases []githubRelease
	err = json.Unmarshal(body, &releases)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}
	return &releases[0], nil
}

// downloadFile 下载文件到本地
func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// unzipFile 解压 zip 文件到目录
func unzipFile(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(destDir, 0o755)
	for _, f := range r.File {
		destPath := filepath.Join(destDir, f.Name)
		// 防止 zip slip
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("zip slip: %s", destPath)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(destPath), 0o755)
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// parseDeltaZip 解析 delta zip 文件，返回 CVE 5.0 Records
func parseDeltaZip(zipPath string) ([]*cveresources.CVERecordV5, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var records []*cveresources.CVERecordV5
	for _, f := range r.File {
		if !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			log.Warnf("open %s in zip failed: %v", f.Name, err)
			continue
		}
		data, err := ioutil.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		var record cveresources.CVERecordV5
		err = json.Unmarshal(data, &record)
		if err != nil {
			continue
		}
		records = append(records, &record)
	}
	return records, nil
}
