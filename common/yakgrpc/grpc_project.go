package yakgrpc

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/bufpipe"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"github.com/yaklang/yaklang/common/gmsm/sm4/padding"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/model"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protowire"

	_ "github.com/mattn/go-sqlite3"
)

var currentProjectMutex = new(sync.Mutex)

func (s *Server) SetCurrentProject(ctx context.Context, req *ypb.SetCurrentProjectRequest) (*ypb.Empty, error) {
	currentProjectMutex.Lock()
	defer currentProjectMutex.Unlock()
	if req.Type == "" { // empty is older front-end default,
		req.Type = yakit.TypeProject
	}

	if req.GetId() <= 0 {
		switch req.GetType() {
		case yakit.TypeProject:
			consts.GetGormProjectDatabase().Close()
		case yakit.TypeSSAProject:
			consts.GetGormSSAProjectDataBase().Close()
		default:
			return nil, utils.Errorf("invalid project type: %s", req.GetType())
		}
		return &ypb.Empty{}, nil
	}

	db := s.GetProfileDatabase()
	proj, err := model.GetProjectById(db, req.GetId())
	if err != nil {
		err := yakit.InitializingProjectDatabase()
		if err != nil {
			log.Errorf("init db failed: %s", err)
		}
		return &ypb.Empty{}, nil
	}
	if proj.Type != req.GetType() {
		return nil, utils.Errorf("type not match %s vs want[%s]", proj.Type, req.GetType())
	}

	err = yakit.SetCurrentProjectById(db, req.GetId())
	if err != nil {
		err := yakit.InitializingProjectDatabase()
		if err != nil {
			log.Errorf("init db failed: %s", err)
		}
		return &ypb.Empty{}, err
	}

	path := proj.DatabasePath
	log.Infof("Set project db by grpc: %s", path)
	switch req.GetType() {
	case yakit.TypeProject:
		consts.SetDefaultYakitProjectDatabaseName(path)
		err = consts.SetGormProjectDatabase(path)
	case yakit.TypeSSAProject:
		raw := proj.DatabasePath
		consts.SetSSADatabaseInfo(raw)
		err = consts.SetGormSSAProjectDatabaseByInfo(raw)
	}
	return &ypb.Empty{}, err
}

func (s *Server) GetCurrentProject(ctx context.Context, _ *ypb.Empty) (*ypb.ProjectDescription, error) {
	return s.GetCurrentProjectEx(ctx, &ypb.GetCurrentProjectExRequest{Type: yakit.TypeProject})
}

func (s *Server) GetCurrentProjectEx(ctx context.Context, req *ypb.GetCurrentProjectExRequest) (*ypb.ProjectDescription, error) {
	currentProjectMutex.Lock()
	defer currentProjectMutex.Unlock()

	db := s.GetProfileDatabase()
	proj, err := yakit.GetCurrentProject(db, req.GetType())
	if err != nil {
		return nil, utils.Errorf("cannot fetch current project")
	}
	return model.ToProjectGRPCModel(proj, consts.GetGormProfileDatabase()), nil
}

func (s *Server) GetProjects(ctx context.Context, req *ypb.GetProjectsRequest) (*ypb.GetProjectsResponse, error) {
	if req.FrontendType == "" {
		req.FrontendType = yakit.TypeProject
	}
	paging, data, err := yakit.QueryProject(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	total, _ := yakit.QueryProjectTotal(s.GetProfileDatabase(), req)
	return &ypb.GetProjectsResponse{
		Projects: funk.Map(data, func(i *schema.Project) *ypb.ProjectDescription {
			return model.ToProjectGRPCModel(i, consts.GetGormProfileDatabase())
		}).([]*ypb.ProjectDescription),
		Pagination:   req.GetPagination(),
		Total:        int64(paging.TotalRecord),
		TotalPage:    int64(paging.Page),
		ProjectToTal: int64(total.TotalRecord),
	}, nil
}

var encryptProjectMagic = []byte{0xff, 0xff, 0xff, 0xff}

func (s *Server) NewProject(ctx context.Context, req *ypb.NewProjectRequest) (*ypb.NewProjectResponse, error) {
	if req.Type == "" {
		return nil, utils.Errorf("type is empty")
	}
	name := req.GetProjectName() // maybe project or folder name
	if err := yakit.CheckInvalidProjectName(name); err != nil {
		return nil, err
	}

	// check this name exist
	pro, _ := yakit.GetProjectByWhere(s.GetProfileDatabase(), req.GetProjectName(), req.GetFolderId(), req.GetChildFolderId(), req.GetType(), req.GetId())
	if pro != nil {
		return nil, utils.Errorf("Project or directory name can not be duplicated in the same directory")
	}

	// check is default project
	if CheckDefault(req.GetProjectName(), req.GetType(), req.GetFolderId(), req.GetChildFolderId()) {
		return nil, utils.Errorf("cannot use this builtin name: %s", yakit.INIT_DATABASE_RECORD_NAME)
	}

	// create project database
	databasePath := req.GetDatabase()
	if databasePath == "" {
		var err error
		databasePath, err = yakit.CreateProjectFile(name, req.GetType())
		if err != nil {
			return nil, utils.Errorf("create project file failed: %v", err)
		}
	}

	// create project in profile database
	projectData := &schema.Project{
		ProjectName:   req.GetProjectName(),
		Description:   req.GetDescription(),
		DatabasePath:  databasePath,
		Type:          req.Type,
		FolderID:      req.FolderId,
		ChildFolderID: req.ChildFolderId,
	}
	if req.ExternalProjectCode != "" {
		projectData.ExternalProjectCode = req.ExternalProjectCode
	}
	if req.ExternalModule != "" {
		projectData.ExternalModule = req.ExternalModule
	}

	// create
	// insert database row
	db := s.GetProfileDatabase()
	if db = db.Create(&projectData); db.Error != nil {
		return nil, db.Error
	}

	return &ypb.NewProjectResponse{Id: int64(projectData.ID), ProjectName: req.GetProjectName()}, nil
}

func (s *Server) UpdateProject(ctx context.Context, req *ypb.NewProjectRequest) (*ypb.NewProjectResponse, error) {
	if req.Type == "" {
		return nil, utils.Errorf("type is empty")
	}
	name := req.GetProjectName() // maybe project or folder name
	if err := yakit.CheckInvalidProjectName(name); err != nil {
		return nil, err
	}

	// create file
	pathName, err := yakit.CreateProjectFile(name, req.GetType())
	if err != nil {
		return nil, utils.Errorf("create project file failed: %v", err)
	}

	if CheckDefault(req.GetProjectName(), req.GetType(), req.GetFolderId(), req.GetChildFolderId()) {
		return nil, utils.Errorf("cannot use this builtin name: %s", yakit.INIT_DATABASE_RECORD_NAME)
	}
	oldPro, err := yakit.GetProjectByID(s.GetProfileDatabase(), req.GetId())
	if err != nil {
		return nil, utils.Errorf("update row not exist: %v", err)
	}

	// if type=file old.databasePath = pathName = ""
	if oldPro.DatabasePath != pathName {
		err = os.Rename(oldPro.DatabasePath, pathName)
		if err != nil {
			return nil, errors.Errorf("rename %s to %s error: %v", oldPro.DatabasePath, pathName, err)
		}
	}

	projectData := schema.Project{
		ProjectName:         req.GetProjectName(),
		Description:         req.GetDescription(),
		DatabasePath:        pathName,
		Type:                req.Type,
		FolderID:            req.FolderId,
		ChildFolderID:       req.ChildFolderId,
		ExternalProjectCode: req.ExternalProjectCode,
		ExternalModule:      req.ExternalModule,
	}
	err = yakit.UpdateProject(s.GetProfileDatabase(), req.GetId(), projectData)
	if err != nil {
		return nil, utils.Errorf("update project failed!")
	}

	return &ypb.NewProjectResponse{Id: int64(req.GetId()), ProjectName: req.GetProjectName()}, nil
}

func (s *Server) IsProjectNameValid(ctx context.Context, req *ypb.IsProjectNameValidRequest) (*ypb.Empty, error) {
	if req.GetType() == "" {
		return nil, utils.Error("type is empty")
	}
	if CheckDefault(req.GetProjectName(), req.GetType(), req.GetFolderId(), req.GetChildFolderId()) {
		return nil, utils.Error("[default] cannot be user's db name")
	}
	proj, _ := yakit.GetProject(consts.GetGormProfileDatabase(), req)
	if proj != nil {
		return nil, utils.Errorf("project name: %s is existed", req.GetProjectName())
	}

	if err := yakit.CheckInvalidProjectName(req.GetProjectName()); err != nil {
		return nil, err
	}

	return &ypb.Empty{}, nil
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
	proj, err := model.GetProjectById(s.GetProfileDatabase(), req.GetId())
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
	outputFile = yakit.GetExportFile(proj.ProjectName, suffix)
	outFp, err := os.OpenFile(outputFile, os.O_CREATE|os.O_RDWR, 0o666)
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
	finished := false
	go func() {
		defer func() {
			finished = true
		}()
		var percent float64 = 0.65
		count := 0
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

	projectReader := io.MultiReader(bytes.NewBuffer(ret), bufio.NewReader(fp))
	fileWriter := gzip.NewWriter(outFp)
	if req.GetPassword() != "" {
		outFp.Write(encryptProjectMagic)
		feedProgress("开始加密数据库... SM4-GCM", 0)
		key := codec.PKCS7Padding([]byte(req.GetPassword()))
		iv := key[:sm4.BlockSize]
		_, err = sm4.Sm4GCMEncryptStream(key, iv, nil, projectReader, fileWriter, padding.NewPKCSPaddingReader)
		if err != nil {
			feedProgress("加密数据库失败/压缩写入项目文件失败:"+err.Error(), 0.97)
			cancel()
			return err
		}
	} else {
		_, err = io.Copy(fileWriter, projectReader)
		if err != nil {
			feedProgress("压缩写入项目文件失败:"+err.Error(), 0.97)
			cancel()
			return err
		}
	}
	fileWriter.Flush()
	fileWriter.Close()

	cancel()
	for !finished {
		time.Sleep(300 * time.Millisecond)
	}
	feedProgress("导出成功", 1.0)
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
			Verbose:    utils.EscapeInvalidUTF8Byte([]byte(verbose)), // avoid invalid utf8 byte for grpc
		})
	}
	if req.GetType() == "" {
		req.Type = yakit.TypeProject // default project
	}

	feedProgress("开始导入项目: "+req.GetLocalProjectName(), 0.1)
	path := req.GetProjectFilePath()
	if !utils.IsFile(path) {
		return utils.Errorf("cannot find local project path: %s", path)
	}

	// build reader -------
	feedProgress("打开项目本地文件:"+req.GetProjectFilePath(), 0.2)
	fp, err := os.Open(req.GetProjectFilePath())
	if err != nil {
		feedProgress("打开项目本地文件失败:"+err.Error(), 0.9)
		return err
	}
	defer fp.Close()
	projectReader := bufio.NewReader(fp)
	feedProgress("正在读取项目文件", 0.3)

	if magicNumber, err := projectReader.Peek(len(encryptProjectMagic)); err != nil {
		feedProgress("读取项目文件失败："+err.Error(), 0.9)
		return err
	} else if bytes.Equal(magicNumber, encryptProjectMagic) {
		if req.GetPassword() != "" {
			_, err = projectReader.Discard(len(encryptProjectMagic))
			if err != nil {
				feedProgress("去除加密幻数失败", 0.99)
				return err
			}
		} else {
			feedProgress("需要密码解密项目数据", 0.99)
			return utils.Error("需要密码解密")
		}
	}

	header, err := projectReader.Peek(3)
	if err != nil {
		return utils.Wrapf(err, "peek header failed")
	}

	var DataReader io.Reader
	var isProjectFile bool
	DataReader = projectReader
	if utils.IsGzip(header) { // if gzip , file is project file
		isProjectFile = true
		DataReader, err = gzip.NewReader(projectReader)
		if err != nil {
			return utils.Wrapf(err, "gzip.NewReader fail")
		}
	}

	// build writer -----
	projectName := utils.TrimFileNameExt(filepath.Base(req.GetProjectFilePath()))
	description := ""
	paramsBytes := make([]byte, 0)
	tempFp, err := os.OpenFile(filepath.Join(consts.GetDefaultYakitProjectsDir(), uuid.NewString()), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		feedProgress("创建临时文件失败："+err.Error(), 0.99)
		return err
	}

	tempFileWriter := bufio.NewWriter(tempFp) // use bufio to improve performance, need separate var to flush before close
	var importProjectWriter io.Writer
	importProjectWriter = tempFileWriter
	if isProjectFile { // Project file should read Proto data
		tryGetProtoData := func(rawBytes []byte) ([]byte, int, bool) {
			data, n := protowire.ConsumeBytes(rawBytes)
			if data != nil && n > 0 {
				return data, n, true
			}
			return nil, 0, false
		}
		importProjectWriter = bufpipe.NewPerHandlerWriter(importProjectWriter, func(i []byte) ([]byte, bool) {
			var n = 0
			var data []byte
			var ok bool
			if data, n, ok = tryGetProtoData(i); !ok {
				return nil, false
			}
			i = i[n:]
			projectName = string(data)
			if data, n, ok = tryGetProtoData(i); !ok {
				return nil, false
			}
			i = i[n:]
			description = string(data)
			if data, n, ok = tryGetProtoData(i); !ok {
				return nil, false
			}
			paramsBytes = data
			return i[n:], true
		})
	}

	feedProgress("正在解压数据库", 0.4)
	if req.GetPassword() != "" {
		feedProgress("解压完成，正在解密数据库", 0.43)
		key := codec.PKCS7Padding([]byte(req.GetPassword()))
		iv := key[:sm4.BlockSize]
		_, err = sm4.Sm4GCMDecryptStream(key, iv, nil, DataReader, importProjectWriter, padding.NewPKCSPaddingWriter)
		if err != nil {
			feedProgress("写入(解密)数据库失败:"+err.Error(), 0.97)
			return err
		}
	} else {
		_, err = io.Copy(importProjectWriter, DataReader)
		if err != nil {
			feedProgress("写入数据库失败:"+err.Error(), 0.97)
			return err
		}
	}
	tempFileWriter.Flush()
	tempFp.Close()

	params := make(map[string]interface{})
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

	_, err = s.IsProjectNameValid(stream.Context(), &ypb.IsProjectNameValidRequest{ProjectName: projectName, Type: req.GetType()})
	if err != nil {
		projectName = projectName + fmt.Sprintf("_%v", utils.RandStringBytes(6))
		_, err := s.IsProjectNameValid(stream.Context(), &ypb.IsProjectNameValidRequest{ProjectName: projectName, Type: req.GetType()})
		if err != nil {
			feedProgress("创建新的项目失败："+projectName+"："+err.Error(), 0.9)
			return utils.Errorf("cannot valid project name: %s", err)
		}
	}
	feedProgress("创建新的项目："+projectName, 0.6)

	fileName, err := yakit.CreateProjectFile(projectName, req.GetType())
	if err != nil {
		feedProgress("创建新数据库失败："+err.Error(), 0.9)
		return err
	}
	err = os.Rename(tempFp.Name(), fileName)
	if err != nil {
		feedProgress("创建新数据库失败："+err.Error(), 0.9)
		return err
	}

	feedProgress("创建项目："+projectName, 0.7)
	proj := &schema.Project{
		ProjectName:   projectName,
		Description:   description,
		DatabasePath:  fileName,
		FolderID:      req.FolderId,
		ChildFolderID: req.GetChildFolderId(),
		Type:          req.GetType(),
	}
	err = yakit.CreateOrUpdateProject(s.GetProfileDatabase(), projectName, req.FolderId, req.ChildFolderId, req.GetType(), proj)
	if err != nil {
		feedProgress("创建项目数据失败："+err.Error(), 0.9)
		return err
	}
	feedProgress("导入项目成功", 1.0)
	return nil
}

func CheckDefault(ProjectName, Type string, FolderId, ChildFolderId int64) bool {
	if ProjectName == yakit.INIT_DATABASE_RECORD_NAME && //  default name
		(Type == yakit.TypeProject || Type == yakit.TypeSSAProject) && // yakit/sast project
		FolderId == 0 && ChildFolderId == 0 { // no parent/child folder
		return true
	}
	return false
}

func (s *Server) DeleteProject(ctx context.Context, req *ypb.DeleteProjectRequest) (*ypb.Empty, error) {
	if req.GetId() == 0 {
		return &ypb.Empty{}, utils.Error("invalid id")
	}

	//  get delete target programs
	db := s.GetProfileDatabase()
	db = db.Where(" id = ? or folder_id = ? or child_folder_id = ? ", req.GetId(), req.GetId(), req.GetId())
	projects := yakit.YieldProject(db, ctx)
	if projects == nil {
		return nil, utils.Error("project is not exist")
	}

	// close current program
	switch req.GetType() {
	case yakit.TypeProject:
		consts.GetGormProjectDatabase().Close()
	case yakit.TypeSSAProject:
		consts.GetGormSSAProjectDataBase().Close()
	}

	// set default to current
	defaultProg, err := s.GetDefaultProjectEx(ctx, &ypb.GetDefaultProjectExRequest{Type: req.GetType()})
	if err != nil {
		return nil, utils.Errorf("get default project err: %v", err)
	}
	s.SetCurrentProject(ctx, &ypb.SetCurrentProjectRequest{Id: defaultProg.Id, Type: req.GetType()})

	// delete selected projects
	for k := range projects {
		if CheckDefault(k.ProjectName, k.Type, k.FolderID, k.ChildFolderID) {
			log.Info("[default] cannot be deleted")
			break
		}
		if req.IsDeleteLocal {
			err := consts.DeleteDatabaseFile(k.DatabasePath)
			if err != nil {
				log.Errorf("delete local database error: %v", err)
			}
		}

		err = yakit.DeleteProjectById(s.GetProfileDatabase(), int64(k.ID))
		if err != nil {
			log.Errorf("delete project error: %v", err)
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) GetDefaultProjectEx(ctx context.Context, req *ypb.GetDefaultProjectExRequest) (*ypb.ProjectDescription, error) {
	proj, err := yakit.GetDefaultProject(s.GetProfileDatabase(), req.GetType())
	if err != nil {
		return nil, utils.Errorf("cannot fetch default project")
	}
	return model.ToProjectGRPCModel(proj, consts.GetGormProfileDatabase()), nil
}
func (s *Server) GetDefaultProject(ctx context.Context, req *ypb.Empty) (*ypb.ProjectDescription, error) {
	return s.GetDefaultProjectEx(ctx, &ypb.GetDefaultProjectExRequest{Type: yakit.TypeProject})
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

func (s *Server) GetTemporaryProjectEx(ctx context.Context, req *ypb.GetTemporaryProjectExRequest) (*ypb.ProjectDescription, error) {
	proj, err := yakit.GetTemporaryProject(s.GetProfileDatabase(), req.GetType())
	if err != nil {
		return nil, utils.Errorf("cannot fetch temporary project")
	}
	return model.ToProjectGRPCModel(proj, consts.GetGormProfileDatabase()), nil
}
func (s *Server) GetTemporaryProject(ctx context.Context, req *ypb.Empty) (*ypb.ProjectDescription, error) {
	return s.GetTemporaryProjectEx(ctx, &ypb.GetTemporaryProjectExRequest{Type: yakit.TypeProject})
}
