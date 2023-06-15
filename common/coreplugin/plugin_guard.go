package coreplugin
//
//import (
//	"github.com/yaklang/yaklang/common/bindata"
//	"github.com/yaklang/yaklang/common/consts"
//	"github.com/yaklang/yaklang/common/log"
//	"github.com/yaklang/yaklang/common/utils"
//	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
//)
//
//func OverWriteCorePluginToLocal() {
//	OverWriteSQLPlugin()
//}
//
//func OverWriteSQLPlugin() {
//	codeBytes, err := bindata.Asset("data/base-yak-plugin/启发式SQL注入检测.yak")
//	if err != nil {
//		log.Error("无法从bindata获取启发式SQL注入检测插件")
//		return
//	}
//	backendSQLSha1 := utils.CalcSha1(string(codeBytes))
//
//	databaseSQLPlugins := yakit.QueryYakScriptByNames(consts.GetGormProfileDatabase(), "启发式SQL注入检测")
//	if len(databaseSQLPlugins) == 0 {
//		log.Info("用户尚未安装启发式SQL注入检测插件，跳过覆写...")
//		return
//	}
//	databaseSQLPlugin := databaseSQLPlugins[0]
//	if databaseSQLPlugin.Content != "" && utils.CalcSha1(databaseSQLPlugin.Content) == backendSQLSha1 {
//		return
//	} else {
//		log.Info("本地数据和后端数据存在差异,后端数据覆写本地数据...")
//		databaseSQLPlugin.Content = string(codeBytes)
//		err := yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), "启发式SQL注入检测", databaseSQLPlugin)
//		if err != nil {
//			log.Error(err)
//			return
//		}
//	}
//}
