package schema

import (
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport/vulnreport"
)

var _ vulnreport.VulnerabilityEntity = (*SSARisk)(nil)

func (s *SSARisk) GetSourceID() string {
	return strconv.FormatUint(uint64(s.ID), 10)
}

func (s *SSARisk) GetHash() string {
	return s.Hash
}

func (s *SSARisk) GetTitle() string {
	return s.Title
}

func (s *SSARisk) GetTitleVerbose() string {
	return s.TitleVerbose
}

func (s *SSARisk) GetSeverity() string {
	return string(s.Severity)
}

func (s *SSARisk) GetRiskType() string {
	return s.RiskType
}

func (s *SSARisk) GetDescription() string {
	return s.Description
}

func (s *SSARisk) GetSolution() string {
	return s.Solution
}

func (s *SSARisk) GetProgramName() string {
	return s.ProgramName
}

func (s *SSARisk) GetCodeSourceURL() string {
	return s.CodeSourceUrl
}

func (s *SSARisk) GetCodeRange() string {
	return s.CodeRange
}

func (s *SSARisk) GetCodeFragment() string {
	return s.CodeFragment
}

func (s *SSARisk) GetFunctionName() string {
	return s.FunctionName
}

func (s *SSARisk) GetLine() int64 {
	return s.Line
}

func (s *SSARisk) GetFromRule() string {
	return s.FromRule
}

func (s *SSARisk) GetCWEList() []string {
	if len(s.CWE) == 0 {
		return nil
	}
	return append([]string(nil), []string(s.CWE)...)
}

func (s *SSARisk) GetTags() []string {
	return utils.StringSplitAndStrip(s.Tags, ",")
}

func (s *SSARisk) GetLatestDisposalStatus() string {
	return s.LatestDisposalStatus
}

func (s *SSARisk) GetLanguage() string {
	return s.Language
}

func (s *SSARisk) GetTaskID() string {
	return ""
}

func (s *SSARisk) GetRuntimeID() string {
	return s.RuntimeId
}

func (s *SSARisk) GetRiskFeatureHash() string {
	return s.RiskFeatureHash
}

func (s *SSARisk) GetCreatedAt() *time.Time {
	if s.CreatedAt.IsZero() {
		return nil
	}
	createdAt := s.CreatedAt.UTC()
	return &createdAt
}

func (s *SSARisk) GetUpdatedAt() *time.Time {
	if s.UpdatedAt.IsZero() {
		return nil
	}
	updatedAt := s.UpdatedAt.UTC()
	return &updatedAt
}

func (s *SSARisk) IsPotentialRisk() bool {
	return s.IsPotential
}
