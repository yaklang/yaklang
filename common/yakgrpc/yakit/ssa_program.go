package yakit

import (
	"sort"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func FilterSSAProgram(db *gorm.DB, filter *ypb.SSAProgramFilter) *gorm.DB {
	db = db.Model(&ssadb.IrProgram{})
	if filter == nil {
		return db
	}

	db = bizhelper.ExactOrQueryStringArrayOr(db, "program_name", filter.GetProgramNames())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "language", filter.GetLanguages())
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetIds())
	// db = bizhelper.ExactOrQueryStringArrayOr(db, "project_name", filter.GetProjectNames())

	projectIds := filter.GetProjectIds()
	if len(projectIds) == 1 && projectIds[0] == 0 {
		db = db.Where("project_id = ? OR project_id IS NULL", 0)
	} else {
		db = bizhelper.ExactQueryUInt64ArrayOr(db, "project_id", projectIds)
	}

	if word := filter.GetKeyword(); word != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"program_name", "description"}, word, false)
	}
	if filter.GetAfterID() > 0 {
		db = db.Where("id > ?", filter.GetAfterID())
	}
	if filter.GetBeforeID() > 0 {
		db = db.Where("id < ?", filter.GetBeforeID())
	}

	if filter.GetBeforeUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", 0, filter.GetBeforeUpdatedAt())
	}
	if filter.GetAfterUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", filter.GetAfterUpdatedAt(), time.Now().Add(10*time.Minute).Unix())
	}
	return db
}

// when filter == nil delete all program
func DeleteSSAProgram(DB *gorm.DB, filter *ypb.SSAProgramFilter) (int, error) {
	// db is profile database
	db := DB.Model(&ssadb.IrProgram{})
	db = FilterSSAProgram(db, filter)
	// get all program
	var programs []*ssadb.IrProgram
	queryResult := db.Model(&ssadb.IrProgram{}).Select("program_name").Find(&programs)
	if queryResult.Error != nil {
		log.Errorf("query ssa program fail: %s", queryResult.Error)
	}
	// delete schema program
	result := db.Unscoped().Delete(&ssadb.IrProgram{})

	// delete ssadb program
	programNames := make([]string, 0, len(programs))
	for _, prog := range programs {
		ssadb.DeleteProgram(ssadb.GetDB(), prog.ProgramName)
		programNames = append(programNames, prog.ProgramName)
	}
	// delete risk create by this program
	DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: programNames})
	// DeleteRisk()
	return len(programs), result.Error
}

// analyzeIncrementalCompileGroups 分析增量编译组
// 返回 map[groupId][]programName，其中 groupId 是最基础的 base program name
// 每个组内的 program 按时间顺序排列，[0] 是最新的（头部）program
func analyzeIncrementalCompileGroups(programs []*ssadb.IrProgram) map[string][]*ssadb.IrProgram {
	groups := make(map[string][]*ssadb.IrProgram)
	programMap := make(map[string]*ssadb.IrProgram) // program name -> program

	// 第一步：建立 program name 到 program 的映射
	for _, prog := range programs {
		if prog.ProgramName != "" {
			programMap[prog.ProgramName] = prog
		}
	}

	// 第二步：确定每个 program 所属的组
	for _, prog := range programs {
		var groupId string

		if prog.IsOverlay && len(prog.OverlayLayers) > 0 {
			// 这是一个 overlay program，groupId 是最基础的 base program（OverlayLayers[0]）
			groupId = prog.OverlayLayers[0]
		} else if prog.BaseProgramName != "" {
			// 这是一个 diff program，需要找到最基础的 base
			groupId = findRootBaseProgram(prog.BaseProgramName, programMap)
		} else {
			// 普通编译的 program，自己就是一个组
			groupId = prog.ProgramName
		}

		// 将 program 添加到对应的组
		if groupId != "" {
			if groups[groupId] == nil {
				groups[groupId] = make([]*ssadb.IrProgram, 0)
			}
			// 检查是否已存在，避免重复
			exists := false
			for _, p := range groups[groupId] {
				if p.ProgramName == prog.ProgramName {
					exists = true
					break
				}
			}
			if !exists {
				groups[groupId] = append(groups[groupId], prog)
			}
		}
	}

	// 第三步：对每个组内的 program 按更新时间排序，最新的在前（[0] 是头部）
	for groupId, groupPrograms := range groups {
		// 按更新时间排序，最新的在前（降序）
		// 使用 sort.Slice 进行降序排序
		sortedPrograms := make([]*ssadb.IrProgram, len(groupPrograms))
		copy(sortedPrograms, groupPrograms)
		sort.Slice(sortedPrograms, func(i, j int) bool {
			return sortedPrograms[i].UpdatedAt.Unix() > sortedPrograms[j].UpdatedAt.Unix()
		})
		groups[groupId] = sortedPrograms
	}

	return groups
}

// findRootBaseProgram 递归查找最基础的 base program
func findRootBaseProgram(baseProgramName string, programMap map[string]*ssadb.IrProgram) string {
	if baseProgramName == "" {
		return ""
	}
	prog, exists := programMap[baseProgramName]
	if !exists {
		// 如果不在 programMap 中，尝试从数据库查询
		prog, err := ssadb.GetApplicationProgram(baseProgramName)
		if err != nil {
			return baseProgramName // 如果查询失败，返回当前名称
		}
		if prog.BaseProgramName == "" {
			return baseProgramName // 没有更基础的 base，返回当前名称
		}
		return findRootBaseProgram(prog.BaseProgramName, programMap)
	}
	if prog.BaseProgramName == "" {
		return baseProgramName // 没有更基础的 base，返回当前名称
	}
	return findRootBaseProgram(prog.BaseProgramName, programMap)
}

func QuerySSAProgram(db *gorm.DB, request *ypb.QuerySSAProgramRequest) (*bizhelper.Paginator, []*ypb.SSAProgram, error) {
	var programs []*ssadb.IrProgram
	p := request.Pagination
	if p == nil {
		p = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)
	db = FilterSSAProgram(db, request.GetFilter())
	paging, dbx := bizhelper.Paging(db, int(p.Page), int(p.Limit), &programs)
	if dbx.Error != nil {
		return nil, nil, utils.Errorf("select ssa program fail: %s", dbx.Error)
	}

	// 分析增量编译组
	groups := analyzeIncrementalCompileGroups(programs)

	// 创建 program 到增量编译信息的映射
	programIncrementalInfo := make(map[string]*incrementalInfo)
	for groupId, groupPrograms := range groups {
		if len(groupPrograms) == 0 {
			continue
		}
		headProgram := groupPrograms[0] // [0] 是头部
		for _, prog := range groupPrograms {
			isIncremental := prog.IsOverlay || prog.BaseProgramName != ""
			var overlayLayers []string
			if prog.IsOverlay && len(prog.OverlayLayers) > 0 {
				overlayLayers = prog.OverlayLayers
			}
			programIncrementalInfo[prog.ProgramName] = &incrementalInfo{
				isIncremental:  isIncremental,
				groupId:         groupId,
				headProgramName: headProgram.ProgramName,
				overlayLayers:   overlayLayers,
			}
		}
	}

	// 对于不在 groups 中的 program（普通编译），也需要设置信息
	for _, prog := range programs {
		if _, exists := programIncrementalInfo[prog.ProgramName]; !exists {
			programIncrementalInfo[prog.ProgramName] = &incrementalInfo{
				isIncremental:  false,
				groupId:         prog.ProgramName,
				headProgramName: prog.ProgramName,
				overlayLayers:   nil,
			}
		}
	}

	progGRPCs := make([]*ypb.SSAProgram, 0, len(programs))
	for _, prog := range programs {
		grpcProg := Prog2GRPC(prog)
		// 填充增量编译信息
		if info, ok := programIncrementalInfo[prog.ProgramName]; ok {
			grpcProg.IsIncrementalCompile = info.isIncremental
			grpcProg.IncrementalGroupId = info.groupId
			grpcProg.HeadProgramName = info.headProgramName
			grpcProg.OverlayLayers = info.overlayLayers
		}
		progGRPCs = append(progGRPCs, grpcProg)
	}
	return paging, progGRPCs, nil
}

// incrementalInfo 存储增量编译信息
type incrementalInfo struct {
	isIncremental  bool
	groupId        string
	headProgramName string
	overlayLayers  []string
}

func QueryLatestSSAProgramNameByProjectId(db *gorm.DB, projectID uint64) (string, error) {
	var names []string
	err := ssadb.GetDB().Model(&ssadb.IrProgram{}).
		Where("project_id = ?", projectID).
		Order("updated_at desc").
		Limit(1).
		Pluck("program_name", &names).Error
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	return names[0], nil
}

// QueryLatestSSAProgramNameByProjectName 通过项目名称查询最新的 program name
func QueryLatestSSAProgramNameByProjectName(db *gorm.DB, projectName string) (string, error) {
	if projectName == "" {
		return "", utils.Errorf("project name is empty")
	}

	// 先通过 project name 查找 project
	var project schema.SSAProject
	err := db.Model(&schema.SSAProject{}).
		Where("project_name = ?", projectName).
		First(&project).Error
	if err != nil {
		return "", utils.Errorf("find project by name %s failed: %s", projectName, err)
	}

	// 通过 project ID 查询最新的 program name
	return QueryLatestSSAProgramNameByProjectId(db, uint64(project.ID))
}

func Prog2GRPC(prog *ssadb.IrProgram) *ypb.SSAProgram {
	ret := &ypb.SSAProgram{
		Id: uint32(prog.ID),
		// basic info
		CreateAt:      prog.CreatedAt.Unix(),
		UpdateAt:      prog.UpdatedAt.Unix(),
		Name:          prog.ProgramName,
		Description:   prog.Description,
		Language:      string(prog.Language),
		EngineVersion: prog.EngineVersion,
		Dbpath:        consts.SSA_PROJECT_DB_RAW,
	}
	// recompile
	NeedReCompile := func() bool {
		return false
		// return prog.EngineVersion != consts.GetYakVersion()
	}
	ret.Recompile = NeedReCompile()
	// risk info
	{
		var result struct {
			High     int64
			Low      int64
			Critical int64
			Middle   int64
			Info     int64
		}
		projectDB := ssadb.GetDB()
		// SFR_SEVERITY "critical" "high" "middle" "low" "info"
		if err := projectDB.Model(&schema.SSARisk{}).Where("program_name=?", prog.ProgramName).Select(`
		sum(case when severity='critical' then 1 else 0 end) as critical,
		sum(case when severity='high' then 1 else 0 end) as high,
		sum(case when severity='middle' then 1 else 0 end) as middle,
		sum(case when severity='low' then 1 else 0 end) as low,
		sum(case when severity='info' then 1 else 0 end) as info	
		`).Scan(&result).Error; err != nil {
			log.Errorf("query risk fail: %s", err) // ignore
		}
		ret.CriticalRiskNumber = result.Critical
		ret.HighRiskNumber = result.High
		ret.WarnRiskNumber = result.Middle
		ret.LowRiskNumber = result.Low
		ret.InfoRiskNumber = result.Info
	}
	ret.SSAProjectID = prog.ProjectID
	return ret
}

func UpdateSSAProgram(DB *gorm.DB, input *ypb.SSAProgramInput) (int64, error) {
	if input == nil {
		return 0, utils.Errorf("input is nil ")
	}
	db := DB.Model(&ssadb.IrProgram{})
	db = db.Where("program_name = ?", input.GetName()).Update("description", input.Description)
	return db.RowsAffected, db.Error
}

func QuerySSAHasNotProjectIDProgram(db *gorm.DB) ([]*ssadb.IrProgram, error) {
	var programs []*ssadb.IrProgram
	if err := db.Model(&ssadb.IrProgram{}).Where("project_id = ? OR project_id IS NULL", 0).Find(&programs).Error; err != nil {
		return nil, utils.Errorf("query programs without project_id failed: %s", err)
	}
	return programs, nil
}

func UpdateIrProgramProjectID(db *gorm.DB, programID uint, projectID uint64) error {
	if programID == 0 {
		return utils.Errorf("program id is required")
	}
	if projectID == 0 {
		return utils.Errorf("project id is required")
	}

	err := db.Model(&ssadb.IrProgram{}).
		Where("id = ?", programID).
		Update("project_id", projectID).Error
	if err != nil {
		return utils.Errorf("update program %d project_id to %d failed: %s", programID, projectID, err)
	}
	return nil
}

func QuerySSACompileTimesByProjectID(db *gorm.DB, projectID uint) int64 {
	var count int64
	db.Model(&ssadb.IrProgram{}).Where("project_id = ?", projectID).Count(&count)
	return count
}

func GetSSAProgramByName(db *gorm.DB, name string) (*ssadb.IrProgram, error) {
	var prog ssadb.IrProgram
	err := db.Model(&ssadb.IrProgram{}).Where("program_name = ?", name).First(&prog).Error
	if err != nil {
		return nil, utils.Errorf("get ssa program by name %s failed: %s", name, err)
	}
	return &prog, nil
}
