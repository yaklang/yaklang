package sfdb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// CreateDefaultRuleLoader 创建默认的规则加载器（数据库）
func CreateDefaultRuleLoader(db *gorm.DB) RuleLoader {
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}
	return NewDBRuleLoader(db)
}

// CreateRuleLoader 根据配置创建规则加载器
// 简化版本：只支持Database和OSS两种类型
func CreateRuleLoader(sourceType RuleSourceType, ossClient OSSClient, db *gorm.DB) RuleLoader {
	switch sourceType {
	case RuleSourceTypeOSS:
		if ossClient == nil {
			log.Warn("oss client is nil, fallback to database loader")
			return CreateDefaultRuleLoader(db)
		}
		log.Info("creating oss rule loader")
		return NewOSSRuleLoader(ossClient)

	case RuleSourceTypeDatabase, "":
		log.Info("creating database rule loader")
		return CreateDefaultRuleLoader(db)

	default:
		log.Warnf("unknown rule source type: %v, using database loader", sourceType)
		return CreateDefaultRuleLoader(db)
	}
}

// CreateOSSRuleLoader 便捷方法：创建OSS规则加载器
func CreateOSSRuleLoader(ossClient OSSClient, bucket, prefix string, enableCache bool) RuleLoader {
	return NewOSSRuleLoader(ossClient,
		WithOSSBucket(bucket),
		WithOSSPrefix(prefix),
		WithOSSCache(enableCache),
	)
}
