package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryCVE(ctx context.Context, req *ypb.QueryCVERequest) (*ypb.QueryCVEResponse, error) {
	paging, data, err := cveresources.QueryCVE(consts.GetGormCVEDatabase(), req)
	if err != nil {
		return nil, err
	}
	var results []*ypb.CVEDetail
	for _, c := range data {
		results = append(results, c.ToGPRCModel())
	}
	return &ypb.QueryCVEResponse{
		Pagination: req.GetPagination(),
		Total:      int64(paging.TotalRecord),
		Data:       results,
	}, nil
}

func (s *Server) GetCVE(ctx context.Context, req *ypb.GetCVERequest) (*ypb.CVEDetailEx, error) {
	if req.GetCVE() == "" {
		return nil, utils.Error("empty filter")
	}

	if db := consts.GetGormCVEDatabase(); db == nil {
		return nil, utils.Error("empty cve database")
	}

	cve, err := cveresources.GetCVE(consts.GetGormCVEDatabase(), req.GetCVE())
	if err != nil {
		return nil, utils.Error("empty cve database")
	}
	var urls []string

	var refArray []map[string]interface{}
	if err := json.Unmarshal(cve.References, &refArray); err == nil {
		for _, rd := range refArray {
			if url, ok := rd["url"].(string); ok {
				urls = append(urls, url)
			}
		}
	} else {
		var ref map[string]interface{}
		err = json.Unmarshal(cve.References, &ref)
		if err != nil {
			log.Errorf("unmarshal references failed: %s", err)
			return nil, err
		}
		if rdArr, ok := ref["reference_data"].([]interface{}); ok {
			for _, rd := range rdArr {
				if rdMap, ok := rd.(map[string]interface{}); ok {
					if url, ok := rdMap["url"].(string); ok {
						urls = append(urls, url)
					}
				}
			}
		}
	}

	urlStr := strings.Join(urls, "\n")
	cve.References = []byte(urlStr)
	var cwes []*ypb.CWEDetail
	f := filter.NewFilter()
	defer f.Close()
	for _, cwe := range utils.PrettifyListFromStringSplitEx(cve.CWE, "|", ",") {
		if strings.HasPrefix(strings.ToLower(cwe), "cwe-") {
			cwe = cwe[4:]
		}

		if f.Exist(cwe) {
			continue
		}
		f.Insert(cwe)
		cweIns, err := cveresources.GetCWE(consts.GetGormCVEDatabase(), cwe)
		if err != nil {
			log.Errorf("get cve failed: %s", err)
			continue
		}
		cwes = append(cwes, cweIns.ToGRPCModel())
	}

	return &ypb.CVEDetailEx{
		CVE: cve.ToGPRCModel(),
		CWE: cwes,
	}, nil
}

func (s *Server) IsCVEDatabaseReady(ctx context.Context, req *ypb.IsCVEDatabaseReadyRequest) (*ypb.IsCVEDatabaseReadyResponse, error) {
	db := consts.GetGormCVEDatabase()
	if db == nil {
		return &ypb.IsCVEDatabaseReadyResponse{
			Ok:     false,
			Reason: "cve database is not found",
		}, nil
	}

	if !db.HasTable("cves") {
		db.Close()
		consts.SetGormCVEDatabase(nil)
		return &ypb.IsCVEDatabaseReadyResponse{
			Ok:     false,
			Reason: "cve database is not found",
		}, nil
	}

	shouldUpdate := false
	_ = shouldUpdate
	var latestRecord cveresources.CVE
	if db := db.Order("last_modified_data DESC").First(&latestRecord); db.Error == nil {
		if latestRecord.LastModifiedData.Before(time.Now().Add(-time.Hour * 24 * 7)) {
			shouldUpdate = true
		}
	} else {
		shouldUpdate = true
	}

	return &ypb.IsCVEDatabaseReadyResponse{
		Ok:           true,
		Reason:       "",
		ShouldUpdate: shouldUpdate,
	}, nil
}

func (s *Server) UpdateCVEDatabase(req *ypb.UpdateCVEDatabaseRequest, stream ypb.Yak_UpdateCVEDatabaseServer) error {
	const targetUrl = "https://cve-db.oss-cn-beijing.aliyuncs.com/default-cve.db.gzip"
	info := func(progress float64, s string, items ...interface{}) {
		var msg string
		if len(items) > 0 {
			msg = fmt.Sprintf(s, items)
		} else {
			msg = s
		}
		log.Info(msg)
		progressInfo, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", progress), 64)
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(msg),
			Progress:  float32(progressInfo),
		})
	}

	if req.GetJustUpdateLatestCVE() {
		client := utils.NewDefaultHTTPClientWithProxy(req.GetProxy())
		client.Timeout = 30 * time.Second

		info(5, "差量更新最新数据：modified/recent nvd db")

		if db := consts.GetGormCVEDatabase(); db == nil {
			info(10, "差量更新数据失败：cve database is not found")
			return nil
		}

		db := consts.GetGormCVEDatabase()
		info(10, "开始下载最新(Recent)数据: Start to download latest CVE Data")
		recent, err := consts.TempFile("cve-recent-*.json.gz")
		if err != nil {
			return err
		}
		recent.Close()
		os.Remove(recent.Name())

		err = utils.DownloadFile(stream.Context(), client, cvequeryops.LatestCveRecentDataFeed, recent.Name(), func(f float64) {
			info((0.1+f*0.2)*100, "下载最新数据中: Downloading Latest CVE Data")
		})
		if err != nil {
			info(10, "下载最新数据失败: Downloading Latest CVE Data Failed: %s", err.Error())
			return err
		}

		modifiedFailed := false
		info(10, "开始下载最新(Modified)数据: Start to download latest CVE Data")
		modified, err := consts.TempFile("cve-modified-*.json.gz")
		if err != nil {
			return err
		}
		modified.Close()
		os.RemoveAll(modified.Name())
		err = utils.DownloadFile(stream.Context(), client, cvequeryops.LatestCveModifiedDataFeed, modified.Name(), func(f float64) {
			info((0.3+f*0.2)*100, "下载最新数据中: Downloading Latest CVE Data")
		})
		if err != nil {
			info(10, "下载最新数据失败: Downloading Latest CVE Data Failed: %s", err.Error())
			modifiedFailed = true
		}

		// load recent
		count := 0
		targetFiles := []string{modified.Name(), recent.Name()}
		if modifiedFailed {
			targetFiles = []string{recent.Name()}
		}
		for index, i := range targetFiles {
			progress := float64(50 + index*20)
			raw, err := ioutil.ReadFile(i)
			if err != nil {
				return err
			}
			raw, err = utils.GzipDeCompress(raw)
			if err != nil {
				return err
			}
			// 解析CVE 2.0格式
			var recentDataV2 cveresources.CVEYearFileV2
			err = json.Unmarshal(raw, &recentDataV2)
			if err != nil {
				info(10, "解压CVE 2.0数据失败: Failed to parse CVE 2.0 format: %s", err.Error())
				return err
			}

			// 处理CVE 2.0格式
			for _, vuln := range recentDataV2.Vulnerabilities {
				cve, err := vuln.ToCVE(db)
				if err != nil {
					log.Error(err)
				}
				if cve != nil {
					count++
					if count%100 == 0 {
						info(progress, "正在更新最新数据(CVE 2.0): Updating Latest CVE Data: %d", count)
					}
					err := cveresources.CreateOrUpdateCVE(db, cve.CVE, cve)
					if err != nil {
						log.Error(err)
					}
				}
			}
		}
		info(100, "更新最新数据: Total: %d", count)
		return nil
	}

	if db := consts.GetGormCVEDatabase(); db != nil {
		info(0, "开始清理旧的 CVE 数据库: Start to clean old CVE Database")
		db.Close()
	}

	os.RemoveAll(consts.GetCVEDatabaseGzipPath())
	os.RemoveAll(consts.GetCVEDatabasePath())
	consts.SetGormCVEDatabase(nil)

	info(0, "开始下载 CVE 数据库: Start to download CVE Database")
	client := utils.NewDefaultHTTPClientWithProxy(req.GetProxy())
	client.Timeout = 30 * time.Minute
	err2 := utils.DownloadFile(stream.Context(), client, targetUrl, consts.GetCVEDatabaseGzipPath(), func(f float64) {
		info(f*100, "下载 CVE 数据库中: Downloading CVE Database")
	})
	if err2 != nil {
		return err2
	}
	info(100, "开始验证数据库加载：Start to verify database")
	db := consts.GetGormCVEDatabase()
	if db == nil {
		info(0, "数据库加载失败! Failed to load database")
		_, err := consts.InitializeCVEDatabase()
		if err != nil {
			info(0, "数据库加载失败（Reason）: %v", err)
			return utils.Errorf("数据库加载失败（Reason）: %s", err)
		}
		return nil
	}
	return nil
}
