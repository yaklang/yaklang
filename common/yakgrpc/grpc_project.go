package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
	"io"
	"io/ioutil"
	"os"
	"yaklang/common/consts"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yakgrpc/yakit"
	"yaklang/common/yakgrpc/ypb"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	currentProjectMutex = new(sync.Mutex)
)

func (s *Server) SetCurrentProject(ctx context.Context, req *ypb.SetCurrentProjectRequest) (*ypb.Empty, error) {
	currentProjectMutex.Lock()
	defer currentProjectMutex.Unlock()
	if req.GetId() > 0 {
		db := s.GetProfileDatabase()
		proj, err := yakit.GetProjectById(db, req.GetId(), yakit.TypeProject)
		if err != nil {
			err := yakit.InitializingProjectDatabase()
			if err != nil {
				log.Errorf("init db failed: %s", err)
			}
			return &ypb.Empty{}, nil
		}
		err = yakit.SetCurrentProjectById(db, req.GetId())
		if err != nil {
			err := yakit.InitializingProjectDatabase()
			if err != nil {
				log.Errorf("init db failed: %s", err)
			}
			return &ypb.Empty{}, nil
		}
		// 不是默认数据库 不需要生成文件
		if CheckDefault(proj.ProjectName, proj.Type, proj.FolderID, proj.ChildFolderID) == nil {
			old, err := os.Open(proj.DatabasePath)
			if err != nil {
				return nil, utils.Errorf("找不到本地数据库文件: %s", err)
			}
			old.Close()
		}

		projectDatabase, err := gorm.Open("sqlite3", proj.DatabasePath)
		if err != nil {
			return nil, utils.Errorf("open project database failed: %s", err)
		}
		projectDatabase.AutoMigrate(yakit.ProjectTables...)
		consts.SetDefaultYakitProjectDatabaseName(proj.DatabasePath)
		consts.SetGormProjectDatabase(projectDatabase)
		return &ypb.Empty{}, nil
	}
	return nil, utils.Errorf("params is empty")
}

func (s *Server) GetProjects(ctx context.Context, req *ypb.GetProjectsRequest) (*ypb.GetProjectsResponse, error) {
	paging, data, err := yakit.QueryProject(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	total, _ := yakit.QueryProjectTotal(s.GetProfileDatabase(), req)
	return &ypb.GetProjectsResponse{
		Projects: funk.Map(data, func(i *yakit.Project) *ypb.ProjectDescription {
			return i.ToGRPCModel()
		}).([]*ypb.ProjectDescription),
		Pagination:   req.GetPagination(),
		Total:        int64(paging.TotalRecord),
		TotalPage:    int64(paging.Page),
		ProjectToTal: int64(total.TotalRecord),
	}, nil
}

var projectNameRe = regexp.MustCompile(`(?i)[_a-z0-9\p{Han}][-_0-9a-z \p{Han}]*`)

func projectNameToFileName(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	return strings.Join(projectNameRe.FindAllString(s, -1), "_")
}

var encryptProjectMagic = []byte{0xff, 0xff, 0xff, 0xff}

func (s *Server) NewProject(ctx context.Context, req *ypb.NewProjectRequest) (*ypb.NewProjectResponse, error) {
	if req.Type == "" {
		return nil, utils.Errorf("type is empty")
	}
	name := req.GetProjectName()
	if !projectNameRe.MatchString(name) {
		return nil, utils.Errorf("create project by name failed! name should match %v", projectNameRe.String())
	}
	var pathName string
	// 项目需要存，文件夹不需要
	if req.Type == yakit.TypeProject {
		databaseName := fmt.Sprintf("yakit-project-%v-%v.sqlite3.db", projectNameToFileName(name), time.Now().Unix())
		pathName = filepath.Join(consts.GetDefaultYakitProjectsDir(), databaseName)
		if ok, _ := utils.PathExists(pathName); ok {
			return nil, utils.Errorf("path: %v is not existed", pathName)
		}
	}
	projectData := &yakit.Project{
		ProjectName:   req.GetProjectName(),
		Description:   req.GetDescription(),
		DatabasePath:  pathName,
		Type:          req.Type,
		FolderID:      req.FolderId,
		ChildFolderID: req.ChildFolderId,
	}
	if req.GetId() > 0 {
		oldPro, err := yakit.GetProjectByID(s.GetProfileDatabase(), req.GetId())
		if err != nil {
			return nil, utils.Errorf("update project not exist %v", err.Error())
		}

		pro, _ := yakit.GetProjectByWhere(s.GetProfileDatabase(), req.GetProjectName(), req.GetFolderId(), req.GetChildFolderId(), req.GetType(), req.GetId())
		if pro != nil {
			return nil, utils.Errorf("同级目录下项目或文件名不能重复")
		}
		err = os.Rename(oldPro.DatabasePath, pathName)
		if err != nil {
			return nil, errors.Errorf(" oldfile=%v rename newname=%v fail=%v", oldPro.DatabasePath, pathName, err)
		}
		err = yakit.UpdateProject(s.GetProfileDatabase(), req.GetId(), *projectData)
		if err != nil {
			return nil, utils.Errorf("update project failed!")
		}

		return &ypb.NewProjectResponse{Id: req.GetId(), ProjectName: req.GetProjectName()}, nil

	} else {
		if CheckDefault(req.GetProjectName(), req.GetType(), req.GetFolderId(), req.GetChildFolderId()) != nil {
			return nil, utils.Errorf("cannot use this name: %s, %v is for buildin", yakit.INIT_DATABASE_RECORD_NAME, yakit.INIT_DATABASE_RECORD_NAME)
		}
	}

	pro, _ := yakit.GetProjectByWhere(s.GetProfileDatabase(), req.GetProjectName(), req.FolderId, req.ChildFolderId, req.Type, 0)
	if pro != nil {
		return nil, utils.Errorf("同级目录下文件/文件夹名不能重复")
	}
	db := s.GetProfileDatabase()
	if db = db.Create(&projectData); db.Error != nil {
		return nil, db.Error
	}
	// 创建库
	projectDatabase, err := gorm.Open("sqlite3", pathName)
	if err != nil {
		return nil, utils.Errorf("open project database failed: %s", err)
	}
	projectDatabase.AutoMigrate(yakit.ProjectTables...)
	projectDatabase.Close()

	return &ypb.NewProjectResponse{Id: int64(projectData.ID), ProjectName: req.GetProjectName()}, nil
}

func (s *Server) RemoveProject(ctx context.Context, req *ypb.RemoveProjectRequest) (*ypb.Empty, error) {
	if req.GetProjectName() == yakit.INIT_DATABASE_RECORD_NAME {
		return nil, utils.Error("[default] cannot be deleted")
	}

	err := yakit.DeleteProjectByProjectName(s.GetProfileDatabase(), req.GetProjectName())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) IsProjectNameValid(ctx context.Context, req *ypb.IsProjectNameValidRequest) (*ypb.Empty, error) {
	if req.GetType() == "" {
		return nil, utils.Error("type is empty")
	}
	if CheckDefault(req.GetProjectName(), req.GetType(), req.GetFolderId(), req.GetChildFolderId()) != nil {
		return nil, utils.Error("[default] cannot be user's db name")
	}
	proj, _ := yakit.GetProject(consts.GetGormProfileDatabase(), req)
	if proj != nil {
		return nil, utils.Errorf("project name: %s is existed", req.GetProjectName())
	}

	if !projectNameRe.MatchString(req.GetProjectName()) {
		return nil, utils.Errorf("validate project by name failed! name should match %v", projectNameRe.String())
	}

	return &ypb.Empty{}, nil
}

func (s *Server) GetCurrentProject(ctx context.Context, _ *ypb.Empty) (*ypb.ProjectDescription, error) {
	currentProjectMutex.Lock()
	defer currentProjectMutex.Unlock()

	db := s.GetProfileDatabase()
	proj, err := yakit.GetCurrentProject(db)
	if err != nil {
		return nil, utils.Errorf("cannot fetch current project")
	}
	return proj.ToGRPCModel(), nil
}

func (s *Server) ExportProject(req *ypb.ExportProjectRequest, stream ypb.Yak_ExportProjectServer) error {
	var outputFile string
	feedProgress := func(verbose string, progress float64) {
		stream.Send(&ypb.ProjectIOProgress{
			TargetPath: outputFile,
			Percent:    progress,
			Verbose:    verbose,
		})
	}
	feedProgress("开始导出", 0.1)

	/*path := consts.GetDefaultYakitProjectDatabase(consts.GetDefaultYakitBaseDir())
	if !utils.IsFile(path) {
		feedProgress("导出失败-"+"数据库不存在："+path, 0.9)
		return utils.Errorf("cannot found database file in: %s", path)
	}*/
	proj, err := yakit.GetProjectById(s.GetProfileDatabase(), req.GetId(), yakit.TypeProject)
	if err != nil {
		feedProgress("导出失败-"+"数据库不存在：", 0.9)
		return utils.Errorf("cannot found database file in: %s", err.Error())
	}
	feedProgress("寻找数据文件", 0.3)
	fp, err := os.Open(proj.DatabasePath)
	if err != nil {
		feedProgress("找不到数据库文件"+err.Error(), 0.4)
		return utils.Errorf("open database failed: %s", err)
	}
	defer fp.Close()

	/*db := s.GetProfileDatabase()
	proj, err := yakit.GetCurrentProject(db)
	if err != nil {
		feedProgress("无法找到当前数据库："+err.Error(), 0.5)
		return err
	}*/

	suffix := ""
	if req.GetPassword() != "" {
		suffix = ".enc"
	}
	outputFile = filepath.Join(consts.GetDefaultYakitProjectsDir(), "project-"+projectNameToFileName(proj.ToGRPCModel().GetProjectName())+".yakitproject"+suffix)
	outFp, err := os.OpenFile(outputFile, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		feedProgress("打开输出文件失败！", 0.5)
		return err
	}
	defer outFp.Close()

	feedProgress("开始导出项目基本数据", 0.6)

	var ret []byte
	ret = protowire.AppendString(ret, proj.ProjectName)
	ret = protowire.AppendString(ret, proj.Description)
	params := map[string]interface{}{
		"allowPassword": req.GetPassword() != "",
	}
	raw, _ := json.Marshal(params)
	ret = protowire.AppendBytes(ret, raw)
	feedProgress("导出项目基本数据成功，开始导出项目数据库", 0.65)

	ctx, cancel := context.WithCancel(context.Background())
	var finished = false
	go func() {
		defer func() {
			finished = true
		}()
		var percent float64 = 0.65
		var count = 0
		for {
			count++
			select {
			case <-ctx.Done():
				return
			default:
				nowPercent := percent + float64(count)*0.01
				if nowPercent > 0.93 {
					return
				}
				feedProgress("", nowPercent)
				time.Sleep(time.Second)
			}
		}
	}()
	var buf bytes.Buffer
	buf.Write(ret)
	io.Copy(&buf, fp)

	var results []byte = buf.Bytes()
	if req.GetPassword() != "" {
		feedProgress("开始加密数据库... SM4-GCM", 0)
		encData, err := codec.SM4GCMEnc(codec.PKCS7Padding([]byte(req.GetPassword())), results, nil)
		if err != nil {
			feedProgress("加密数据库失败:"+err.Error(), 0.97)
			cancel()
			return err
		}
		results = encData
	}

	feedProgress("开始压缩数据库", 0)
	results, err = utils.GzipCompress(results)
	if err != nil {
		feedProgress("导出项目失败：GZIP 压缩失败: "+err.Error(), 0.97)
		cancel()
		return err
	}

	if req.GetPassword() != "" {
		feedProgress("开始写入加密数据，请妥善保管密码", 0.94)
	}

	if req.GetPassword() != "" {
		outFp.Write(encryptProjectMagic)
	}
	outFp.Write(results)
	cancel()
	for !finished {
		time.Sleep(300 * time.Millisecond)
	}
	feedProgress("导出成功，导出项目大小："+utils.ByteSize(uint64(len(results))), 1.0)
	return nil
}

func (s *Server) MigrateLegacyDatabase(ctx context.Context, req *ypb.Empty) (*ypb.Empty, error) {
	err := yakit.MigrateLegacyDatabase()
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) ImportProject(req *ypb.ImportProjectRequest, stream ypb.Yak_ImportProjectServer) error {
	feedProgress := func(verbose string, progress float64) {
		stream.Send(&ypb.ProjectIOProgress{
			TargetPath: req.GetProjectFilePath(),
			Percent:    progress,
			Verbose:    verbose,
		})
	}

	feedProgress("开始导入项目: "+req.GetLocalProjectName(), 0.1)
	path := req.GetProjectFilePath()
	if !utils.IsFile(path) {
		return utils.Errorf("cannot find local project path: %s", path)
	}

	feedProgress("打开项目本地文件:"+req.GetProjectFilePath(), 0.2)
	fp, err := os.Open(req.GetProjectFilePath())
	if err != nil {
		feedProgress("打开项目本地文件失败:"+err.Error(), 0.9)
		return err
	}
	defer fp.Close()

	feedProgress("正在读取项目文件", 0.3)
	raw, err := ioutil.ReadAll(fp)
	if err != nil {
		feedProgress("读取项目文件失败："+err.Error(), 0.9)
		return err
	}

	if bytes.HasPrefix(raw, encryptProjectMagic) {
		if req.GetPassword() != "" {
			raw = raw[len(encryptProjectMagic):]
		} else {
			feedProgress("需要密码解密项目数据", 0.99)
			return utils.Error("需要密码解密")
		}
	}

	feedProgress("正在解压数据库", 0.4)
	bytes, err := utils.GzipDeCompress(raw)
	if err != nil {
		return err
	}

	feedProgress("解压完成，正在解密数据库", 0.43)
	if req.GetPassword() != "" {
		decData, err := codec.SM4GCMDec(codec.PKCS7Padding([]byte(req.GetPassword())), bytes, nil)
		if err != nil {
			feedProgress("解密失败！", 0.99)
			return utils.Error("解密失败！")
		}
		bytes = decData
	}

	feedProgress("读取项目基本信息", 0.45)
	projectName, n := protowire.ConsumeString(bytes)
	bytes = bytes[n:]
	description, n := protowire.ConsumeString(bytes)
	bytes = bytes[n:]
	paramsBytes, n := protowire.ConsumeBytes(bytes)
	bytes = bytes[n:]

	var params = make(map[string]interface{})
	json.Unmarshal(paramsBytes, &params)
	if params != nil && len(params) > 0 {
		// handle params
	}

	feedProgress(fmt.Sprintf(
		"读取项目基本信息，原始项目名「%v」，描述信息：%v",
		projectName, description,
	), 0.5)

	if req.GetLocalProjectName() != "" {
		projectName = req.GetLocalProjectName()
	}

	if projectName == "[default]" {
		projectName = "_default_"
	}

	_, err = s.IsProjectNameValid(stream.Context(), &ypb.IsProjectNameValidRequest{ProjectName: projectName, Type: yakit.TypeProject})
	if err != nil {
		projectName = projectName + fmt.Sprintf("_%v", utils.RandStringBytes(6))
		_, err := s.IsProjectNameValid(stream.Context(), &ypb.IsProjectNameValidRequest{ProjectName: projectName})
		if err != nil {
			feedProgress("创建新的项目失败："+projectName+"："+err.Error(), 0.9)
			return utils.Errorf("cannot valid project name: %s", err)
		}
	}
	feedProgress("创建新的项目："+projectName, 0.6)
	databaseName := fmt.Sprintf("yakit-%v-%v.sqlite3.db", projectNameToFileName(projectName), time.Now().Unix())
	fileName := filepath.Join(consts.GetDefaultYakitProjectsDir(), databaseName)
	err = os.WriteFile(
		fileName,
		bytes,
		0666,
	)
	if err != nil {
		feedProgress("创建新数据库失败："+err.Error(), 0.9)
		return err
	}

	feedProgress("创建项目："+projectName, 0.7)
	proj := &yakit.Project{
		ProjectName:   projectName,
		Description:   description,
		DatabasePath:  fileName,
		FolderID:      req.FolderId,
		ChildFolderID: req.GetChildFolderId(),
		Type:          "project",
	}
	err = yakit.CreateOrUpdateProject(s.GetProfileDatabase(), projectName, req.FolderId, req.ChildFolderId, "project", proj)
	if err != nil {
		feedProgress("创建项目数据失败："+err.Error(), 0.9)
		return err
	}
	feedProgress("导入项目成功", 1.0)
	return nil
}

func CheckDefault(ProjectName, Type string, FolderId, ChildFolderId int64) error {
	if ProjectName == yakit.INIT_DATABASE_RECORD_NAME && Type == yakit.TypeProject && FolderId == 0 && ChildFolderId == 0 {
		return utils.Error("[default] cannot be deleted")
	}
	return nil
}

func (s *Server) DeleteProject(ctx context.Context, req *ypb.DeleteProjectRequest) (*ypb.Empty, error) {
	if req.GetId() > 0 {
		db := s.GetProfileDatabase()
		db = db.Where(" id = ? or folder_id = ? or child_folder_id = ? ", req.GetId(), req.GetId(), req.GetId())
		projects := yakit.YieldProject(db.Debug(), ctx)
		if projects == nil {
			return nil, utils.Error("删除项目不存在")
		}
		for k := range projects {
			if CheckDefault(k.ProjectName, k.Type, k.FolderID, k.ChildFolderID) != nil {
				log.Info("[default] cannot be deleted")
				break
			}
			if req.IsDeleteLocal {
				err := os.RemoveAll(k.DatabasePath)
				if err != nil {
					log.Error("删除本地数据库失败：" + err.Error())
				}
			}
			err := yakit.DeleteProjectById(s.GetProfileDatabase(), int64(k.ID))
			if err != nil {
				log.Error("删除项目失败：" + err.Error())
			}
		}
		return &ypb.Empty{}, nil
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetDefaultProject(ctx context.Context, req *ypb.Empty) (*ypb.ProjectDescription, error) {
	proj, err := yakit.GetDefaultProject(s.GetProfileDatabase())
	if err != nil {
		return nil, utils.Errorf("cannot fetch default project")
	}
	return proj.ToGRPCModel(), nil
}

func (s *Server) QueryProjectDetail(ctx context.Context, req *ypb.QueryProjectDetailRequest) (*ypb.ProjectDescription, error) {
	var proj *ypb.ProjectDescription
	if req.GetId() > 0 {
		proj, err := yakit.GetProjectDetail(s.GetProfileDatabase(), req.GetId())
		if err != nil {
			return nil, utils.Errorf("cannot fetch project")
		}
		return proj.BackGRPCModel(), nil
	}
	return proj, nil
}
