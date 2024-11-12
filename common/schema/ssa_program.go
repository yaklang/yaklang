package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSAProgram struct {
	gorm.Model

	Name        string `json:"name" gorm:"type:varchar(255);unique_index"`
	Description string `json:"description" gorm:"type:text"`

	DBPath string `json:"db_path"`
	// program language when set
	Language      string `json:"language" gorm:"type:varchar(255)"`
	EngineVersion string `json:"engine_version" gorm:"type:varchar(255)"`
}

func (s *SSAProgram) ToGrpcProgram() *ypb.SsaProgram {
	return &ypb.SsaProgram{
		CreateTime:    s.CreatedAt.String(),
		Name:          s.Name,
		Description:   s.Description,
		Dbpath:        s.DBPath,
		Language:      s.Language,
		EngineVersion: s.EngineVersion,
	}
}
func ToSchemaSsaProgram(program *ypb.SsaProgram) *SSAProgram {
	return &SSAProgram{
		Name:          program.Name,
		Description:   program.Description,
		DBPath:        program.Dbpath,
		Language:      program.Language,
		EngineVersion: program.EngineVersion,
	}
}
