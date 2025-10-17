package generate_index_tool

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
)

// FileCacheManager 基于文件的缓存管理器
type FileCacheManager struct {
	cacheDir string
}

// NewFileCacheManager 创建文件缓存管理器
func NewFileCacheManager(cacheDir string) *FileCacheManager {
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}
	return &FileCacheManager{
		cacheDir: cacheDir,
	}
}

// LoadRawCache 加载原始内容缓存
func (f *FileCacheManager) LoadRawCache() (map[string]string, error) {
	return f.loadCache("raw_content.json")
}

// SaveRawCache 保存原始内容缓存
func (f *FileCacheManager) SaveRawCache(cache map[string]string) error {
	return f.saveCache("raw_content.json", cache)
}

// LoadProcessedCache 加载处理后内容缓存
func (f *FileCacheManager) LoadProcessedCache() (map[string]string, error) {
	return f.loadCache("processed_content.json")
}

// SaveProcessedCache 保存处理后内容缓存
func (f *FileCacheManager) SaveProcessedCache(cache map[string]string) error {
	return f.saveCache("processed_content.json", cache)
}

// Clear 清空所有缓存
func (f *FileCacheManager) Clear() error {
	files := []string{"raw_content.json", "processed_content.json"}
	for _, file := range files {
		path := filepath.Join(f.cacheDir, file)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return utils.Errorf("删除缓存文件失败 %s: %v", path, err)
		}
	}
	return nil
}

// loadCache 加载缓存文件
func (f *FileCacheManager) loadCache(filename string) (map[string]string, error) {
	path := filepath.Join(f.cacheDir, filename)

	// 如果文件不存在，返回空缓存
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return make(map[string]string), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, utils.Errorf("读取缓存文件失败 %s: %v", path, err)
	}

	var cache map[string]string
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, utils.Errorf("解析缓存文件失败 %s: %v", path, err)
	}

	return cache, nil
}

// saveCache 保存缓存文件
func (f *FileCacheManager) saveCache(filename string, cache map[string]string) error {
	// 确保缓存目录存在
	if err := os.MkdirAll(f.cacheDir, 0755); err != nil {
		return utils.Errorf("创建缓存目录失败 %s: %v", f.cacheDir, err)
	}

	path := filepath.Join(f.cacheDir, filename)

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return utils.Errorf("序列化缓存数据失败: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return utils.Errorf("写入缓存文件失败 %s: %v", path, err)
	}

	return nil
}
