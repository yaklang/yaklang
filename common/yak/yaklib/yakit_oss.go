package yaklib

import (
	"context"
	"io"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// OSSClient OSS客户端接口
// 兼容多种OSS服务（阿里云OSS、AWS S3、MinIO等）
type OSSClient interface {
	// ListObjects 列出bucket中指定前缀的对象
	// bucket: bucket名称
	// prefix: 对象前缀
	// 返回: 对象列表和错误
	ListObjects(bucket, prefix string) ([]OSSObject, error)

	// GetObject 获取对象内容
	// bucket: bucket名称
	// key: 对象key
	// 返回: 对象内容和错误
	GetObject(bucket, key string) ([]byte, error)

	// GetObjectStream 获取对象内容流（用于大文件）
	// bucket: bucket名称
	// key: 对象key
	// 返回: 读取器和错误
	GetObjectStream(bucket, key string) (io.ReadCloser, error)

	// Close 关闭客户端
	Close() error

	// GetType 返回OSS类型
	GetType() OSSType
}

// OSSObject OSS对象信息
type OSSObject struct {
	Key          string // 对象Key
	Size         int64  // 对象大小（字节）
	LastModified int64  // 最后修改时间（Unix时间戳）
	ETag         string // ETag
}

// OSSType OSS服务类型
type OSSType string

const (
	OSSTypeAliyun OSSType = "aliyun" // 阿里云OSS
	OSSTypeS3     OSSType = "s3"     // AWS S3
	OSSTypeMinIO  OSSType = "minio"  // MinIO
	OSSTypeCustom OSSType = "custom" // 自定义S3兼容
)

// String 返回OSS类型的字符串表示
func (t OSSType) String() string {
	return string(t)
}

// IsValid 检查OSS类型是否有效
func (t OSSType) IsValid() bool {
	switch t {
	case OSSTypeAliyun, OSSTypeS3, OSSTypeMinIO, OSSTypeCustom:
		return true
	default:
		return false
	}
}

// OSSConfig OSS配置
type OSSConfig struct {
	Endpoint        string  // OSS endpoint
	AccessKeyID     string  // Access Key ID
	AccessKeySecret string  // Access Key Secret
	Bucket          string  // Bucket名称
	Prefix          string  // 规则文件前缀
	Region          string  // 区域（AWS S3需要）
	OSSType         OSSType // OSS类型
	EnableSSL       bool    // 是否启用SSL
}

// AliyunOSSClient 阿里云OSS客户端
type AliyunOSSClient struct {
	client *oss.Client
}

// NewAliyunOSSClient 创建阿里云OSS客户端
// endpoint: OSS endpoint，例如 "oss-cn-hangzhou.aliyuncs.com"
// accessKeyID: Access Key ID
// accessKeySecret: Access Key Secret
func NewAliyunOSSClient(endpoint, accessKeyID, accessKeySecret string) (*AliyunOSSClient, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret, oss.EnableCRC(true))
	if err != nil {
		return nil, utils.Errorf("create aliyun oss client failed: %w", err)
	}
	return &AliyunOSSClient{
		client: client,
	}, nil
}

// NewAliyunOSSClientWithConfig 使用配置创建阿里云OSS客户端
func NewAliyunOSSClientWithConfig(config *OSSConfig) (*AliyunOSSClient, error) {
	return NewAliyunOSSClient(config.Endpoint, config.AccessKeyID, config.AccessKeySecret)
}

// ListObjects 列出bucket中指定前缀的对象
func (c *AliyunOSSClient) ListObjects(bucket, prefix string) ([]OSSObject, error) {
	bucketIns, err := c.client.Bucket(bucket)
	if err != nil {
		return nil, utils.Errorf("get bucket failed: %w", err)
	}

	objects := make([]OSSObject, 0)

	// 使用 ListObjects 方法，传入选项和回调函数
	marker := ""
	for {
		result, err := bucketIns.ListObjects(oss.Prefix(prefix), oss.MaxKeys(1000), oss.Marker(marker))
		if err != nil {
			return nil, utils.Errorf("list objects failed: %w", err)
		}

		// 添加当前页的对象
		for _, obj := range result.Objects {
			objects = append(objects, OSSObject{
				Key:          obj.Key,
				Size:         obj.Size,
				LastModified: obj.LastModified.Unix(),
				ETag:         obj.ETag,
			})
		}

		// 检查是否还有更多对象
		if !result.IsTruncated {
			break
		}
		marker = result.NextMarker
	}

	return objects, nil
}

// GetObject 获取对象内容
func (c *AliyunOSSClient) GetObject(bucket, key string) ([]byte, error) {
	bucketIns, err := c.client.Bucket(bucket)
	if err != nil {
		return nil, utils.Errorf("get bucket failed: %w", err)
	}

	body, err := bucketIns.GetObject(key)
	if err != nil {
		return nil, utils.Errorf("get object %s failed: %w", key, err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, utils.Errorf("read object %s failed: %w", key, err)
	}

	return data, nil
}

// GetObjectStream 获取对象内容流
func (c *AliyunOSSClient) GetObjectStream(bucket, key string) (io.ReadCloser, error) {
	bucketIns, err := c.client.Bucket(bucket)
	if err != nil {
		return nil, utils.Errorf("get bucket failed: %w", err)
	}

	body, err := bucketIns.GetObject(key)
	if err != nil {
		return nil, utils.Errorf("get object %s failed: %w", key, err)
	}

	return body, nil
}

// Close 关闭客户端
func (c *AliyunOSSClient) Close() error {
	// 阿里云OSS客户端没有显式关闭方法
	return nil
}

// GetType 返回OSS类型
func (c *AliyunOSSClient) GetType() OSSType {
	return OSSTypeAliyun
}

// DownloadOSSRules 从OSS下载规则并保存到数据库
// 这是高级封装，类似于 Save 方法的功能
// ctx: 上下文
// ossClient: OSS客户端
// bucket: bucket名称
// prefix: 规则文件前缀
func DownloadOSSRules(ctx context.Context, ossClient OSSClient, bucket, prefix string) error {
	log.Infof("Starting to download OSS rules from bucket=%s, prefix=%s", bucket, prefix)

	stream := DownloadOSSSyntaxFlowRuleFiles(ctx, ossClient, bucket, prefix)

	successCount := 0
	errorCount := 0

	for item := range stream.Chan {
		if item.Error != nil {
			log.Warnf("download OSS rule file error: %v", item.Error)
			errorCount++
			continue
		}

		// 这里可以添加保存到数据库的逻辑
		// 为了保持 yaklib 包不导入 sfdb，此逻辑应该在调用方完成
		log.Infof("Downloaded rule: %s (size: %d bytes)", item.RuleName, len(item.Content))
		successCount++
	}

	log.Infof("OSS rules download completed: success=%d, failed=%d", successCount, errorCount)

	if successCount == 0 {
		return utils.Errorf("failed to download any rules: failed=%d", errorCount)
	}

	return nil
}

// TestOSSConnection 测试OSS连接
// ossClient: OSS客户端
// bucket: bucket名称
// 返回: 错误信息
func TestOSSConnection(ossClient OSSClient, bucket string) error {
	if ossClient == nil {
		return utils.Error("OSS client is nil")
	}
	if bucket == "" {
		return utils.Error("bucket name is empty")
	}

	_, err := ossClient.ListObjects(bucket, "")
	if err != nil {
		return utils.Errorf("test oss connection failed: %w", err)
	}
	return nil
}

// GetOSSClientInfo 获取OSS客户端信息
// ossClient: OSS客户端
// 返回: OSS类型和错误
func GetOSSClientInfo(ossClient OSSClient) (OSSType, error) {
	if ossClient == nil {
		return "", utils.Error("OSS client is nil")
	}
	return ossClient.GetType(), nil
}
