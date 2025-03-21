package schema

import (
	"github.com/jinzhu/gorm"
	"testing"
)

func TestRisk_ColorizedShow(t *testing.T) {
	type fields struct {
		Model               gorm.Model
		Hash                string
		IP                  string
		IPInteger           int64
		Url                 string
		Port                int
		Host                string
		Title               string
		TitleVerbose        string
		Description         string
		Solution            string
		RiskType            string
		RiskTypeVerbose     string
		Parameter           string
		Payload             string
		Details             string
		Severity            string
		FromYakScript       string
		YakScriptUUID       string
		WaitingVerified     bool
		ReverseToken        string
		RuntimeId           string
		QuotedRequest       string
		QuotedResponse      string
		IsPotential         bool
		CVE                 string
		IsRead              bool
		Ignore              bool
		UploadOnline        bool
		TaskName            string
		CveAccessVector     string
		CveAccessComplexity string
		Tags                string
		ResultID            uint
		Variable            string
		ProgramName         string
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			fields: fields{
				Title: "test",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Risk{
				Model:               tt.fields.Model,
				Hash:                tt.fields.Hash,
				IP:                  tt.fields.IP,
				IPInteger:           tt.fields.IPInteger,
				Url:                 tt.fields.Url,
				Port:                tt.fields.Port,
				Host:                tt.fields.Host,
				Title:               tt.fields.Title,
				TitleVerbose:        tt.fields.TitleVerbose,
				Description:         tt.fields.Description,
				Solution:            tt.fields.Solution,
				RiskType:            tt.fields.RiskType,
				RiskTypeVerbose:     tt.fields.RiskTypeVerbose,
				Parameter:           tt.fields.Parameter,
				Payload:             tt.fields.Payload,
				Details:             tt.fields.Details,
				Severity:            tt.fields.Severity,
				FromYakScript:       tt.fields.FromYakScript,
				YakScriptUUID:       tt.fields.YakScriptUUID,
				WaitingVerified:     tt.fields.WaitingVerified,
				ReverseToken:        tt.fields.ReverseToken,
				RuntimeId:           tt.fields.RuntimeId,
				QuotedRequest:       tt.fields.QuotedRequest,
				QuotedResponse:      tt.fields.QuotedResponse,
				IsPotential:         tt.fields.IsPotential,
				CVE:                 tt.fields.CVE,
				IsRead:              tt.fields.IsRead,
				Ignore:              tt.fields.Ignore,
				UploadOnline:        tt.fields.UploadOnline,
				TaskName:            tt.fields.TaskName,
				CveAccessVector:     tt.fields.CveAccessVector,
				CveAccessComplexity: tt.fields.CveAccessComplexity,
				Tags:                tt.fields.Tags,
				ResultID:            tt.fields.ResultID,
				Variable:            tt.fields.Variable,
				ProgramName:         tt.fields.ProgramName,
			}
			p.ColorizedShow()
		})
	}
}
