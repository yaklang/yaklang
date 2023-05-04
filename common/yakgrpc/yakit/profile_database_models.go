package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"sync"
)

var initUserDataAndPluginOnce = new(sync.Once)

// ProfileTables 这些表是独立与项目之外的，每一个用户的数据都不一样
var ProfileTables = []interface{}{
	&YakScript{}, &Payload{}, &MenuItem{},
	&GeneralStorage{}, &MarkdownDoc{},
	&Project{},
}

// ProjectTables 这些表是和项目关联的，导出项目可以直接复制给用户
var ProjectTables = []interface{}{
	&WebsocketFlow{},
	&HTTPFlow{}, &ExecHistory{},
	&ExtractedData{},
	&Port{},
	&Domain{}, &Host{},
	&MarkdownDoc{}, &ExecResult{},
	&Risk{}, &WebFuzzerTask{}, &WebFuzzerResponse{},
	&ReportRecord{}, &ScreenRecorder{},

	// rss
	&Briefing{}, &RssFeed{},
	// &assets.SubscriptionSource{},
}

func UserDataAndPluginDatabaseScope(db *gorm.DB) *gorm.DB {
	initUserDataAndPluginOnce.Do(func() {
		if d := consts.GetGormProfileDatabase(); d != nil {
			d.AutoMigrate(ProfileTables...)
		}
	})
	return consts.GetGormProfileDatabase()
}
