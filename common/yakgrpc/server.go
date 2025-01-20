package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools/dicts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Server struct {
	ypb.YakServer
	homeDir            string
	cacheDir           string
	_abandonedDatabase *gorm.DB
	reverseServer      *facades.FacadeServer
	profileDatabase    *gorm.DB
	projectDatabase    *gorm.DB
}

type ServerOpts func(config *ServerConfig)

type ServerConfig struct {
	reverseServerPort   int
	initFacadeServer    bool
	startCacheLog       bool
	profileDatabasePath string
	projectDatabasePath string
}

func WithReverseServerPort(port int) ServerOpts {
	return func(config *ServerConfig) {
		config.reverseServerPort = port
	}
}

func WithInitFacadeServer(init bool) ServerOpts {
	return func(config *ServerConfig) {
		config.initFacadeServer = init
	}
}

func WithStartCacheLog() ServerOpts {
	return func(config *ServerConfig) {
		config.startCacheLog = true
	}
}

func WithProfileDatabasePath(p string) ServerOpts {
	return func(config *ServerConfig) {
		config.profileDatabasePath = p
	}
}

func WithProjectDatabasePath(p string) ServerOpts {
	return func(config *ServerConfig) {
		config.projectDatabasePath = p
	}
}

func (s *Server) GetProfileDatabase() *gorm.DB {
	if s != nil && s.profileDatabase != nil {
		return s.profileDatabase
	}
	return consts.GetGormProfileDatabase()
}

func (s *Server) GetProjectDatabase() *gorm.DB {
	if s != nil && s.projectDatabase != nil {
		return s.projectDatabase
	}
	return consts.GetGormProjectDatabase()
}

func NewServer(opts ...ServerOpts) (*Server, error) {
	return NewServerWithLogCache(opts...)
}

func NewTestServer() (*Server, error) {
	// return newServerEx(false, startCacheLog)
	return newServerEx(
		WithStartCacheLog(),
		WithInitFacadeServer(false),
	)
}

func NewServerWithLogCache(opts ...ServerOpts) (*Server, error) {
	// return newServerEx(true, startCacheLog)
	return newServerEx(opts...)
}

func newServerEx(opts ...ServerOpts) (*Server, error) {
	serverConfig := &ServerConfig{
		initFacadeServer: true,
	}
	for _, opt := range opts {
		opt(serverConfig)
	}

	yakitBase := consts.GetDefaultYakitBaseDir()
	_ = os.MkdirAll(yakitBase, 0o777)
	s := &Server{
		cacheDir: yakitBase,
	}

	if len(serverConfig.profileDatabasePath) > 0 {
		db, err := consts.CreateProfileDatabase(serverConfig.profileDatabasePath)
		if err != nil {
			return nil, err
		}
		s.profileDatabase = db
	}

	if len(serverConfig.projectDatabasePath) > 0 {
		db, err := consts.CreateProjectDatabase(serverConfig.projectDatabasePath)
		if err != nil {
			return nil, err
		}
		s.projectDatabase = db
	}

	if serverConfig.initFacadeServer {
		// if serverConfig.reverseServerPort == 0 {
		// 	port, err = utils.GetRangeAvailableTCPPort(50000, 65535, 3)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// } else {
		// 	if utils.IsTCPPortAvailable(serverConfig.reverseServerPort) && !utils.IsTCPPortOpen("127.0.0.1", serverConfig.reverseServerPort) {
		// 		port = serverConfig.reverseServerPort
		// 	} else {
		// 		return nil, utils.Errorf("this port has used: %v", port)
		// 	}
		// }
		s.reverseServer = facades.NewFacadeServer("0.0.0.0", 0)
	}

	err := s.init()
	if err != nil {
		return nil, err
	}
	if serverConfig.startCacheLog {
		utils.StartCacheLog(context.Background(), 200)
	}
	return s, nil
}

var YakitProfileTables = schema.ProfileTables

var YakitAllTables = schema.ProjectTables

func (s *Server) init() error {
	var err error

	//fd, err := os.Stat(s.cacheDir)
	//if err != nil {
	//	return err
	//}
	//howToFix := func() {
	//	log.Infof("数据库无写权限，请检查 [%v]", s.defaultDatabaseFile)
	//	if strings.Contains(strings.ToLower(fd.Mode().String()), "rw") {
	//		log.Info("可能的问题：数据库文件/或数据库文件所属的目录 owner 不匹配")
	//		log.Info("perhaps: the owner for [database file / resource dir] error")
	//		log.Info("解决方案(1)/try: chown -R [your-user-name] ~/yakit-projects")
	//		log.Info("解决方案(2)/or try: rm -rf ~/yakit-projects")
	//	}
	//}
	//
	//log.Infof("database dir[%v] mode: %v", s.cacheDir, fd.Mode())
	//fp, err := os.OpenFile(s.defaultDatabaseFile, os.O_RDWR, os.ModePerm)
	//if err != nil {
	//	log.Errorf("cannot open database for %v", err)
	//	howToFix()
	//	return err
	//}
	//fp.Close()
	err = s.initBasicData()
	if err != nil {
		log.Warnf("init database failed: %s", err)
		// howToFix()
		return err
	}

	err = s.initFacadeServer()
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) initFacadeServer() error {
	if s.reverseServer == nil {
		return nil
	}

	s.reverseServer.OnHandle(func(n *facades.Notification) {
		res, ok := remoteAddrConvertor.Get(n.RemoteAddr)
		if ok {
			n.RemoteAddr = res
		}
		_, _ = yakit.NewRisk(
			n.RemoteAddr,
			yakit.WithRiskParam_Title(fmt.Sprintf("reverse [%v] connection from %s", n.Type, n.RemoteAddr)),
			yakit.WithRiskParam_TitleVerbose(fmt.Sprintf(`接收到来自 [%v] 的反连[%v]`, n.RemoteAddr, n.Type)),
			yakit.WithRiskParam_RiskType(fmt.Sprintf(`reverse-%v`, n.Type)),
			yakit.WithRiskParam_Details(n),
			yakit.WithRiskParam_Token(n.Token),
		)
	})
	s.reverseServer.RemoteAddrConvertorHandler = func(s string) string {
		res, ok := remoteAddrConvertor.Get(s)
		if !ok {
			return s
		}
		return res
	}
	go func() {
		_run := func() {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()

			err := s.reverseServer.ServeWithContext(context.Background())
			if err != nil {
				log.Error(err)
				return
			}
		}
		for {
			_run()
			time.Sleep(500 * time.Millisecond)
		}
	}()
	return nil
}

func (s *Server) initBasicData() error {
	// 初始化各种 payload
	var err error

	// 检查版本，如果版本和当前版本不匹配，自动删除缓存
	EngineVersionKey := "_YAKLANG_ENGINE_VERSION"
	if yakit.GetKey(s.GetProfileDatabase(), EngineVersionKey) != consts.GetYakVersion() {
		yakit.SetKey(s.GetProfileDatabase(), EngineVersionKey, consts.GetYakVersion())
		log.Info("yaklang core engine version changed... remove cache!")
		_ = os.RemoveAll(consts.GetDefaultYakitBaseTempDir())
	} else {
		log.Infof("yaklang core engine version: %v, cache is working!", consts.GetYakVersion())
	}

	if yakit.GetPayloadCountInGroup(s.GetProfileDatabase(), `user_top10`) <= 0 {
		err = yakit.SavePayloadGroup(s.GetProfileDatabase(), "user_top10", dicts.UsernameTop10)
		if err != nil {
			return err
		}
	}

	if yakit.GetPayloadCountInGroup(s.GetProfileDatabase(), `pass_top25`) <= 0 {
		err = yakit.SavePayloadGroup(s.GetProfileDatabase(), "pass_top25", dicts.PasswordTop25)
		if err != nil {
			return err
		}
	}
	return nil
}
