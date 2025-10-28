package sfdb

import (
	"io"
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
