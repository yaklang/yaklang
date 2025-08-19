package aireactdeps

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// initializeDatabase 初始化Yakit数据库和配置
func initializeDatabase() error {
	log.Info("Initializing Yakit database and configurations...")

	// 初始化Yakit数据库（项目和配置文件）
	consts.InitializeYakitDatabase("", "", "")

	// 初始化CVE数据库（可选，如果不工作不要失败）
	_, err := consts.InitializeCVEDatabase()
	if err != nil {
		log.Warnf("Failed to initialize CVE database: %v", err)
	}

	// 调用数据库初始化后的函数（网络配置等）
	err = yakit.CallPostInitDatabase()
	if err != nil {
		log.Warnf("Failed to call post-init database functions: %v", err)
		return err
	}

	log.Info("Database and configurations initialized successfully")
	return nil
}
