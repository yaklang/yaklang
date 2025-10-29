package yaklib

import (
	"bytes"
	"io"

	"github.com/yaklang/yaklang/common/utils"
)

// MockOSSClient Mock OSS客户端，用于测试
type MockOSSClient struct {
	objects map[string][]byte // key -> content
	ossType OSSType
}

// NewMockOSSClient 创建Mock OSS客户端
func NewMockOSSClient(ossType OSSType) *MockOSSClient {
	return &MockOSSClient{
		objects: make(map[string][]byte),
		ossType: ossType,
	}
}

// AddObject 添加对象到Mock客户端
func (c *MockOSSClient) AddObject(key string, content []byte) {
	c.objects[key] = content
}

// AddRuleObject 添加规则对象到Mock客户端
func (c *MockOSSClient) AddRuleObject(ruleName, ruleContent string) {
	key := "syntaxflow/" + ruleName + ".sf"
	c.objects[key] = []byte(ruleContent)
}

// ListObjects 列出对象
func (c *MockOSSClient) ListObjects(bucket, prefix string) ([]OSSObject, error) {
	objects := make([]OSSObject, 0)

	for key := range c.objects {
		if prefix == "" || utils.MatchAllOfGlob(key, prefix+"*") {
			objects = append(objects, OSSObject{
				Key:          key,
				Size:         int64(len(c.objects[key])),
				LastModified: 0,
				ETag:         "",
			})
		}
	}

	return objects, nil
}

// GetObject 获取对象内容
func (c *MockOSSClient) GetObject(bucket, key string) ([]byte, error) {
	content, ok := c.objects[key]
	if !ok {
		return nil, utils.Errorf("object %s not found", key)
	}
	return content, nil
}

// GetObjectStream 获取对象内容流
func (c *MockOSSClient) GetObjectStream(bucket, key string) (io.ReadCloser, error) {
	content, err := c.GetObject(bucket, key)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

// Close 关闭客户端
func (c *MockOSSClient) Close() error {
	return nil
}

// GetType 返回OSS类型
func (c *MockOSSClient) GetType() OSSType {
	return c.ossType
}
