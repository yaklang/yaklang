package airaghttp

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// 在线知识库列表索引与下载根地址
// 关键词: online rag list, oss-qn rags-latest.json, build-in knowledge base
const (
	onlineRagListLink = "https://oss-qn.yaklang.com/rag/rags-latest.json"
	onlineRagBaseURL  = "https://oss-qn.yaklang.com"
)

// OnlineRagInfo 在线知识库条目 (与 rags-latest.json 字段对应)
// 关键词: BuildInRagInfo, name_zh, file_size, hash
type OnlineRagInfo struct {
	Name     string `json:"name"`
	NameZh   string `json:"name_zh"`
	Version  string `json:"version"`
	File     string `json:"file"`
	FileSize int64  `json:"file_size"`
	HashFile string `json:"hashfile"`
	HashType string `json:"hashtype"`
	Hash     string `json:"hash"`
}

// DownloadURL 返回该知识库的完整下载地址
func (i *OnlineRagInfo) DownloadURL() string {
	return onlineRagBaseURL + i.File
}

// Filename 返回下载到本地的文件名
func (i *OnlineRagInfo) Filename() string {
	return filepath.Base(i.File)
}

// ListOnlineRags 拉取在线可下载知识库列表
// 关键词: rag-list, fetch rags-latest.json
func ListOnlineRags() ([]*OnlineRagInfo, error) {
	isHttps, getRequest, err := lowhttp.ParseUrlToHttpRequestRaw("GET", onlineRagListLink)
	if err != nil {
		return nil, utils.Errorf("parse online rag list url failed: %v", err)
	}

	rsp, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes([]byte(getRequest)),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithSaveHTTPFlow(false),
	)
	if err != nil {
		return nil, utils.Errorf("fetch online rag list failed: %v", err)
	}

	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp.RawPacket)

	var infos []*OnlineRagInfo
	if err := json.Unmarshal(body, &infos); err != nil {
		return nil, utils.Errorf("parse online rag list failed: %v", err)
	}
	return infos, nil
}

// findOnlineRag 根据中英文名称匹配在线知识库
func findOnlineRag(infos []*OnlineRagInfo, name string) *OnlineRagInfo {
	for _, info := range infos {
		if info.Name == name || info.NameZh == name {
			return info
		}
	}
	return nil
}

// DefaultRagDownloadDir 返回默认下载目录 ~/yakit-projects/libs
func DefaultRagDownloadDir() string {
	return filepath.Join(consts.GetDefaultYakitProjectsDir(), "libs")
}

// DownloadRagProgress 下载进度回调
type DownloadRagProgress func(name string, percent float64, message string)

// DownloadRag 下载单个知识库并导入到 profile DB
// 关键词: rag-download, thirdparty_bin.DownloadFile, rag.ImportRAG
func DownloadRag(ctx context.Context, name string, force bool, downloadDir string, onProgress DownloadRagProgress) error {
	if downloadDir == "" {
		downloadDir = DefaultRagDownloadDir()
	}

	infos, err := ListOnlineRags()
	if err != nil {
		return err
	}

	info := findOnlineRag(infos, name)
	if info == nil {
		return utils.Errorf("online rag not found: %s", name)
	}

	if onProgress != nil {
		onProgress(info.Name, 0, "start downloading")
	}
	log.Infof("downloading rag %s from %s", info.Name, info.DownloadURL())

	filePath, err := thirdparty_bin.DownloadFile(info.DownloadURL(), info.Filename(), downloadDir, &thirdparty_bin.InstallOptions{
		Context: ctx,
		Force:   force,
		Progress: func(percent float64, downloaded, total int64, message string) {
			if onProgress != nil {
				onProgress(info.Name, percent*80, "downloading")
			}
		},
	})
	if err != nil {
		return utils.Errorf("download rag %s failed: %v", info.Name, err)
	}

	if onProgress != nil {
		onProgress(info.Name, 80, "importing into database")
	}
	log.Infof("importing rag %s into profile database", info.Name)

	importErr := rag.ImportRAG(filePath,
		rag.WithDB(consts.GetGormProfileDatabase()),
		rag.WithRAGCtx(ctx),
		rag.WithName(info.Name),
		rag.WithExportOverwriteExisting(force),
		rag.WithRAGSerialVersionUID(info.Hash),
		rag.WithProgressHandler(func(percent float64, message string, messageType string) {
			if onProgress != nil {
				onProgress(info.Name, 80+percent*0.2, message)
			}
		}),
	)
	if importErr != nil {
		return utils.Errorf("import rag %s failed: %v", info.Name, importErr)
	}

	if onProgress != nil {
		onProgress(info.Name, 100, "download and import finished")
	}
	log.Infof("rag %s downloaded and imported successfully", info.Name)
	return nil
}

// DownloadAllRags 下载并导入全部在线知识库
// 关键词: rag-download --all, batch download
func DownloadAllRags(ctx context.Context, force bool, downloadDir string, onProgress DownloadRagProgress) error {
	infos, err := ListOnlineRags()
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		return utils.Error("no online rag available to download")
	}

	var lastErr error
	for _, info := range infos {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e := DownloadRag(ctx, info.Name, force, downloadDir, onProgress); e != nil {
			log.Errorf("download rag %s failed: %v", info.Name, e)
			lastErr = e
		}
	}
	return lastErr
}
