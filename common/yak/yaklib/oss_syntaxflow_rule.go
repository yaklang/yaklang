package yaklib

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// OSSRuleFileDownloadStream OSS规则文件下载流
type OSSRuleFileDownloadStream struct {
	Total int64
	Chan  chan *OSSRuleFileItem
}

// OSSRuleFileItem OSS规则文件项（原始内容，未解析）
type OSSRuleFileItem struct {
	RuleName string // 规则名称（从文件名提取）
	Content  string // 规则内容（.sf 文件原始内容）
	Key      string // OSS 对象 key
	Error    error  // 错误
}

// DownloadOSSSyntaxFlowRuleFiles 从OSS下载SyntaxFlow规则文件
// 注意：此函数只下载原始 .sf 文件内容，不解析规则
// 解析和保存的逻辑应该在调用方完成（避免循环导入）
//
// 参数:
//   - ctx: 上下文
//   - ossClient: OSS客户端
//   - bucket: bucket名称
//   - prefix: 规则文件前缀（如 "syntaxflow/"）
//
// 返回: 规则文件下载流
func DownloadOSSSyntaxFlowRuleFiles(
	ctx context.Context,
	ossClient OSSClient,
	bucket string,
	prefix string,
) *OSSRuleFileDownloadStream {
	ch := make(chan *OSSRuleFileItem, 10)
	rsp := &OSSRuleFileDownloadStream{
		Total: 0,
		Chan:  ch,
	}

	go func() {
		defer close(ch)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("recover from DownloadOSSSyntaxFlowRuleFiles panic: %v", err)
			}
		}()

		// 1. 列出所有 .sf 文件
		objects, err := ossClient.ListObjects(bucket, prefix)
		if err != nil {
			ch <- &OSSRuleFileItem{
				Error: utils.Wrapf(err, "list objects from OSS failed"),
			}
			return
		}

		// 过滤出 .sf 文件
		sfFiles := make([]OSSObject, 0)
		for _, obj := range objects {
			if strings.HasSuffix(obj.Key, ".sf") {
				sfFiles = append(sfFiles, obj)
			}
		}

		rsp.Total = int64(len(sfFiles))
		log.Infof("Found %d .sf files in OSS bucket %s with prefix %s", len(sfFiles), bucket, prefix)

		// 2. 逐个下载规则文件
		for i, obj := range sfFiles {
			select {
			case <-ctx.Done():
				log.Info("context cancelled, stop downloading rules from OSS")
				return
			default:
			}

			log.Infof("[%d/%d] Downloading: %s", i+1, len(sfFiles), obj.Key)

			// 下载文件内容
			content, err := ossClient.GetObject(bucket, obj.Key)
			if err != nil {
				ch <- &OSSRuleFileItem{
					Error: utils.Wrapf(err, "get object %s failed", obj.Key),
				}
				continue
			}

			// 提取规则名称
			ruleName := extractRuleNameFromKey(obj.Key, prefix)

			// 发送原始内容（不解析）
			select {
			case ch <- &OSSRuleFileItem{
				RuleName: ruleName,
				Content:  string(content),
				Key:      obj.Key,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return rsp
}

// extractRuleNameFromKey 从对象key中提取规则名称
// 例如: "syntaxflow/java/sql-injection.sf" -> "sql-injection"
//
//	"syntaxflow/php/xss-check.sf" -> "xss-check"
func extractRuleNameFromKey(key string, prefix string) string {
	// 移除前缀
	name := strings.TrimPrefix(key, prefix)

	// 移除 .sf 后缀
	name = strings.TrimSuffix(name, ".sf")

	// 移除路径分隔符，只保留文件名
	parts := strings.Split(name, "/")
	if len(parts) > 0 {
		name = parts[len(parts)-1]
	}

	// 如果为空，使用完整路径（去掉后缀）
	if name == "" {
		name = strings.TrimSuffix(key, ".sf")
		name = strings.ReplaceAll(name, "/", "_")
	}

	return name
}
