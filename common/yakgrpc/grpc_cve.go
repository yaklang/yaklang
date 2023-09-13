package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
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
	var ref map[string]interface{}
	err = json.Unmarshal(cve.References, &ref)
	if err != nil {
		log.Errorf("unmarshal references failed: %s", err)
		return nil, err
	}
	// 获取每个URL字段并将其拼接为一个字符串
	var urls []string
	if rdArr, ok := ref["reference_data"].([]interface{}); ok {
		for _, rd := range rdArr {
			if rdMap, ok := rd.(map[string]interface{}); ok {
				if url, ok := rdMap["url"].(string); ok {
					urls = append(urls, url)
				}
			}
		}
	}
	urlStr := strings.Join(urls, "\n")
	cve.References = []byte(urlStr)
	var cwes []*ypb.CWEDetail
	f := filter.NewFilter()
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
		client := netx.NewDefaultHTTPClient(req.GetProxy())
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

		err = utils.DownloadFile(client, cvequeryops.LatestCveRecentDataFeed, recent.Name(), func(f float64) {
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
		err = utils.DownloadFile(client, cvequeryops.LatestCveModifiedDataFeed, modified.Name(), func(f float64) {
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
			var recentData cveresources.CVEYearFile
			err = json.Unmarshal(raw, &recentData)
			if err != nil {
				info(10, "解压最新数据失败: Decompress Latest CVE Data Failed: %s", err.Error())
				return err
			}
			for _, i := range recentData.CVERecords {

				cve, err := i.ToCVE(db)
				if err != nil {
					log.Error(err)
				}
				if cve != nil {
					count++
					if count%100 == 0 {
						info(progress, "正在更新最新数据: Updating Latest CVE Data: %d", count)
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
	client := netx.NewDefaultHTTPClient(req.GetProxy())
	client.Timeout = 30 * time.Minute

	info(0, "获取下载材料大小: Fetching Download Material Basic Info")
	rsp, err := client.Head(targetUrl)
	if err != nil {
		// 提示勿动
		return utils.Errorf("client failed: %s", err)
	}

	i, err := strconv.Atoi(rsp.Header.Get("Content-Length"))
	if err != nil {
		return utils.Errorf("cannot fetch cl: %v", err)
	}
	info(0, "共需下载大小为：Download %v Total", utils.ByteSize(uint64(i)))

	rsp, err = client.Get(targetUrl)
	if err != nil {
		return utils.Errorf("download db failed: %s", err)
	}

	fp, err := os.OpenFile(consts.GetCVEDatabaseGzipPath(), os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return utils.Errorf("open gzip file failed: %s", err)
	}

	prog := progresswriter.New(uint64(i))
	go func() {
		for {
			time.Sleep(time.Second)
			select {
			case <-stream.Context().Done():
				return
			default:
				info(prog.GetPercent()*100, "")
				if prog.GetPercent() >= 1 {
					return
				}
			}
		}
	}()
	_, err = io.Copy(fp, io.TeeReader(rsp.Body, prog))
	if err != nil {
		fp.Close()
		info(0, "下载文件失败: Download Failed: %s", err)
		return utils.Errorf("下载文件失败: Download Failed: %s", err)
	}
	fp.Close()
	info(100, "下载文件成功：Download Finished")

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
