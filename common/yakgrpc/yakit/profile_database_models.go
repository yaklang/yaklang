package yakit

import (
	"github.com/yaklang/yaklang/common/consts"
)

// ProfileTables 这些表是独立与项目之外的，每一个用户的数据都不一样
var ProfileTables = []interface{}{
	&YakScript{}, &Payload{}, &MenuItem{},
	&GeneralStorage{}, &MarkdownDoc{},
	&Project{},
	&NavigationBar{}, &NaslScript{},
	&WebFuzzerLabel{},
	&PluginGroup{},
	&CodecFlow{},
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
	&ProjectGeneralStorage{},
	// rss
	&Briefing{}, &RssFeed{}, &WebShell{},
	// &assets.SubscriptionSource{},
	&AliveHost{},

	// traffic
	&TrafficSession{}, &TrafficPacket{}, &TrafficTCPReassembledFrame{},

	// HybridScan
	&HybridScanTask{},

	// Progress
	&Progress{},
}

func init() {
	consts.RegisterDatabaseSchema(consts.KEY_SCHEMA_YAKIT_DATABASE, ProjectTables...)
	consts.RegisterDatabaseSchema(consts.KEY_SCHEMA_PROFILE_DATABASE, ProfileTables...)
}
