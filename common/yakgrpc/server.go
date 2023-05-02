package yakgrpc

import (
	"context"
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"time"
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
}

func (*Server) GetProfileDatabase() *gorm.DB {
	return consts.GetGormProfileDatabase()
}

func (*Server) GetProjectDatabase() *gorm.DB {
	return consts.GetGormProjectDatabase()
}

func NewServer() (*Server, error) {
	yakitBase := consts.GetDefaultYakitBaseDir()
	_ = os.MkdirAll(yakitBase, 0777)
	s := &Server{
		cacheDir:      yakitBase,
		reverseServer: facades.NewFacadeServer("0.0.0.0", utils.GetRandomAvailableTCPPort()),
	}

	err := s.initDatabase()
	if err != nil {
		return nil, err
		//log.Warnf("cannot fetch database connection: %v", err)
		//log.Infof("checking your [%v] 's fs.permission", yakitBase)
		//
		//// 不存在数据
		//if utils.GetFirstExistedPath(yakitBase) != yakitBase {
		//	return nil, utils.Errorf("yakit-projects non-existed.")
		//}
		//
		//f, err := os.Stat(yakitBase)
		//if err != nil {
		//	return nil, err
		//}
		//
		//log.Info("数据库遭遇问题/ database met error")
		//log.Infof("dir: %v mode: %v", yakitBase, f.Mode().String())
		//log.Infof("尝试/try 0755. (至少对当前用户需要rwx权限 / chmod 0755 %v)", yakitBase)
		//log.Infof(
		//	"or... checking owner for %v (检查 *nix 系统下 %v 的 owner 是否为当前用户, 通过 chown -R [your-user] ~/yakit-projects)",
		//	yakitBase, yakitBase,
		//)
		//
		//err2 := os.RemoveAll(consts.GetDefaultYakitBaseDir(homeDir))
		//if err2 != nil {
		//	log.Error("remove %v failed: %s", yakitBase, err2)
		//	return nil, err2
		//}
		//
		//_ = os.MkdirAll(yakitBase, os.ModePerm)
		//err = s.initDatabase()
		//if err != nil {
		//	return nil, err
		//}
	}
	return s, nil
}

var YakitProfileTables = yakit.ProfileTables

var YakitAllTables = yakit.ProjectTables

func (s *Server) initDatabase() error {
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
		//howToFix()
		return err
	}

	err = s.initFacadeServer()
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) initFacadeServer() error {
	s.reverseServer.OnHandle(func(n *facades.Notification) {
		res, ok := remoteAddrConvertor.Get(n.RemoteAddr)
		if ok {
			n.RemoteAddr = fmt.Sprint(res)
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
		return fmt.Sprint(res)
	}
	go func() {
		var _run = func() {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()

			log.Infof("serve reverse(facade) server...")
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

	if yakit.GetPayloadCount(s.GetProfileDatabase(), `user_top10`) <= 0 {
		err = yakit.SavePayloadGroup(s.GetProfileDatabase(), "user_top10", dicts.UsernameTop10)
		if err != nil {
			return err
		}
	}

	if yakit.GetPayloadCount(s.GetProfileDatabase(), `pass_top25`) <= 0 {
		err = yakit.SavePayloadGroup(s.GetProfileDatabase(), "pass_top25", dicts.PasswordTop25)
		if err != nil {
			return err
		}
	}
	return nil
}
