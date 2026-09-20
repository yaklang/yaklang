package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
)

const ssaIRProgramDeleteResultPrefix = "legion.realtime.ssa.ir_program.delete"

type ssaIRProgramDeleteRequest struct {
	RequestID     string `json:"request_id"`
	StoreIdentity string `json:"store_identity"`
	ProgramName   string `json:"program_name"`
}

type ssaIRProgramDeleteResponse struct {
	RequestID     string `json:"request_id"`
	StoreIdentity string `json:"store_identity"`
	ProgramName   string `json:"program_name"`
	Success       bool   `json:"success"`
	Deleted       bool   `json:"deleted"`
	AlreadyAbsent bool   `json:"already_absent,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type ssaIRProgramDeleteFunc func(context.Context, string, string) (deleted bool, alreadyAbsent bool, reason string, err error)
type ssaIRProgramDeletePublishFunc func(context.Context, string, ssaIRProgramDeleteResponse) error

func (b *legionJobBridge) handleSSAIRProgramDelete(ctx context.Context, raw []byte) error {
	deleteProgram := deleteSSAIRProgramForLifecycle
	if b != nil && b.agent != nil && b.agent.manager != nil {
		deleteProgram = func(
			ctx context.Context,
			storeIdentity string,
			programName string,
		) (bool, bool, string, error) {
			release, ok := b.agent.manager.HoldIdle()
			if !ok {
				return false, false, "", errors.New("扫描节点仍有任务运行，IR 程序未删除")
			}
			defer release()
			return deleteSSAIRProgramForLifecycle(ctx, storeIdentity, programName)
		}
	}
	return handleSSAIRProgramDeleteWith(
		ctx,
		raw,
		deleteProgram,
		b.publishSSAIRProgramDeleteResponse,
	)
}

func handleSSAIRProgramDeleteWith(
	ctx context.Context,
	raw []byte,
	deleteProgram ssaIRProgramDeleteFunc,
	publish ssaIRProgramDeletePublishFunc,
) error {
	var request ssaIRProgramDeleteRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return fmt.Errorf("unmarshal ssa IR program delete: %w", err)
	}
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.StoreIdentity = strings.TrimSpace(request.StoreIdentity)
	request.ProgramName = strings.TrimSpace(request.ProgramName)
	if request.RequestID == "" {
		return errors.New("ssa IR program delete request_id is required")
	}
	if request.ProgramName == "" {
		return errors.New("ssa IR program delete program_name is required")
	}
	if request.StoreIdentity == "" {
		return errors.New("ssa IR program delete store_identity is required")
	}
	if deleteProgram == nil || publish == nil {
		return errors.New("ssa IR program delete handler is not configured")
	}

	deleted, alreadyAbsent, reason, deleteErr := deleteProgram(ctx, request.StoreIdentity, request.ProgramName)
	response := ssaIRProgramDeleteResponse{
		RequestID:     request.RequestID,
		StoreIdentity: request.StoreIdentity,
		ProgramName:   request.ProgramName,
		Success:       deleteErr == nil,
		Deleted:       deleted,
		AlreadyAbsent: alreadyAbsent,
		Reason:        strings.TrimSpace(reason),
	}
	if deleteErr != nil {
		response.Reason = deleteErr.Error()
	}
	// A completed refusal is a terminal response, not a transport failure. The
	// platform receives the reason and decides how to report or retry it.
	return publish(ctx, request.RequestID, response)
}

func deleteSSAIRProgramForLifecycle(
	ctx context.Context,
	expectedStoreIdentity string,
	programName string,
) (bool, bool, string, error) {
	if err := ctx.Err(); err != nil {
		return false, false, "", err
	}
	db := ssadb.GetDB()
	if db == nil {
		return false, false, "", errors.New("SSA IR database is not configured")
	}
	actualStoreIdentity, err := ssaIRStoreIdentity(db)
	if err != nil {
		return false, false, "", fmt.Errorf("resolve SSA IR store identity: %w", err)
	}
	if strings.TrimSpace(expectedStoreIdentity) != actualStoreIdentity {
		return false, false, "", errors.New("IR 存储身份不匹配，未删除")
	}
	var target ssadb.IrProgram
	if err := db.Where("program_name = ?", programName).First(&target).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return false, true, "IR 程序已不存在", nil
		}
		return false, false, "", fmt.Errorf("query IR program: %w", err)
	}

	baseName := ssaproject.BaseProjectNameFromProgramName(programName)
	var programs []ssadb.IrProgram
	if err := db.Select("id, program_name, updated_at").Find(&programs).Error; err != nil {
		return false, false, "", fmt.Errorf("verify IR baseline: %w", err)
	}
	newerExists := false
	for _, candidate := range programs {
		if candidate.ProgramName == target.ProgramName ||
			ssaproject.BaseProjectNameFromProgramName(candidate.ProgramName) != baseName {
			continue
		}
		if candidate.UpdatedAt.After(target.UpdatedAt) ||
			(candidate.UpdatedAt.Equal(target.UpdatedAt) && candidate.ID > target.ID) {
			newerExists = true
			break
		}
	}
	if !newerExists {
		return false, false, "", errors.New("该 IR 程序仍是当前增量编译基线，未删除")
	}
	if err := ssadb.DeleteProgramChecked(db, programName); err != nil {
		return false, false, "", fmt.Errorf("delete IR program: %w", err)
	}
	var remaining int
	if err := db.Model(&ssadb.IrProgram{}).Where("program_name = ?", programName).Count(&remaining).Error; err != nil {
		return false, false, "", fmt.Errorf("verify deleted IR program: %w", err)
	}
	if remaining != 0 {
		return false, false, "", errors.New("IR 程序删除后仍可读取，未确认删除成功")
	}
	return true, false, "IR 程序已删除", nil
}

func ssaIRStoreIdentity(db *gorm.DB) (string, error) {
	if db == nil || db.Dialect() == nil {
		return "", errors.New("SSA IR database dialect is unavailable")
	}
	dialect := strings.ToLower(strings.TrimSpace(db.Dialect().GetName()))
	switch dialect {
	case "postgres", "postgresql":
		var systemIdentifier string
		var databaseName string
		row := db.Raw(`
SELECT system_identifier::text, current_database()::text
FROM pg_catalog.pg_control_system()
`).Row()
		if err := row.Scan(&systemIdentifier, &databaseName); err != nil {
			return "", fmt.Errorf("read PostgreSQL cluster identity: %w", err)
		}
		systemIdentifier = strings.TrimSpace(systemIdentifier)
		databaseName = strings.TrimSpace(databaseName)
		if systemIdentifier == "" || databaseName == "" {
			return "", errors.New("PostgreSQL cluster identity is incomplete")
		}
		return buildSSAIRStoreIdentity("postgres", systemIdentifier, databaseName), nil
	case "sqlite", "sqlite3":
		rows, err := db.Raw("PRAGMA database_list").Rows()
		if err != nil {
			return "", fmt.Errorf("read SQLite database identity: %w", err)
		}
		defer rows.Close()
		var databasePath string
		for rows.Next() {
			var sequence int
			var name string
			var path string
			if err := rows.Scan(&sequence, &name, &path); err != nil {
				return "", fmt.Errorf("scan SQLite database identity: %w", err)
			}
			if name == "main" {
				databasePath = strings.TrimSpace(path)
				break
			}
		}
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("iterate SQLite database identity: %w", err)
		}
		if databasePath == "" {
			return "", errors.New("in-memory SQLite IR stores do not have a stable identity")
		}
		absolutePath, err := filepath.Abs(databasePath)
		if err != nil {
			return "", fmt.Errorf("resolve SQLite database path: %w", err)
		}
		return buildSSAIRStoreIdentity("sqlite", filepath.Clean(absolutePath)), nil
	default:
		return "", fmt.Errorf("unsupported SSA IR database dialect %q", dialect)
	}
}

func buildSSAIRStoreIdentity(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("ir-store-v1:%x", digest)
}

func (b *legionJobBridge) publishSSAIRProgramDeleteResponse(
	ctx context.Context,
	requestID string,
	response ssaIRProgramDeleteResponse,
) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal ssa IR program delete response: %w", err)
	}
	publisher, ok := b.capabilityPublisher.(*capabilityEventPublisher)
	if !ok || publisher == nil {
		return errors.New("capability event publisher is not ready")
	}
	publishCtx := ctx
	if b != nil && b.agent != nil && b.agent.node != nil {
		publishCtx = b.agent.node.GetRootContext()
	}
	publishCtx, cancel := context.WithTimeout(publishCtx, 5*time.Second)
	defer cancel()
	return publisher.PublishRaw(
		publishCtx,
		ssaIRProgramDeleteResultPrefix+"."+requestID,
		raw,
	)
}
