package mcp

import (
	"context"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("cve",
		WithTool(mcp.NewTool("query_cve",
			mcp.WithDescription("Queries CVE based on flexible filters"),
			mcp.WithPaging("pagination",
				[]string{"id", "created_at", "updated_at", "deleted_at", "cve", "cwe", "problem_type", "references", "title_zh", "solution", "description_main", "description_main_zh", "descriptions", "vendor", "product", "cpe_configurations", "cvss_version", "cvss_vector_string", "access_vector", "access_complexity", "authentication", "confidentiality_impact", "integrity_impact", "availability_impact", "base_cvs_sv2_score", "severity", "exploitability_score", "impact_score", "obtain_all_privilege", "obtain_user_privilege", "obtain_other_privilege", "user_interaction_required", "published_date", "last_modified_data"},
				mcp.Description("Pagination settings for the query"),
			),
			mcp.WithString("cve",
				mcp.Description("CVE identifier"),
			),
			mcp.WithString("accessVector",
				mcp.Description("Access vector of the vulnerability"),
				mcp.Enum("NETWORK", "LOCAL", "ADJACENT_NETWORK", "PHYSICAL"),
			),
			mcp.WithString("accessComplexity",
				mcp.Description("Access complexity of the vulnerability"),
				mcp.Enum("HIGH", "MIDDLE", "LOW"),
			),
			mcp.WithString("cwe",
				mcp.Description("Common Weakness Enumeration identifier"),
			),
			mcp.WithString("year",
				mcp.Description("Year of the CVE publication"),
			),
			mcp.WithString("severity",
				mcp.Description("Severity level of the vulnerability"),
				mcp.Enum("HIGH", "MIDDLE", "LOW"),
			),
			mcp.WithNumber("score",
				mcp.Description("Common Vulnerability Scoring System 2.0 score"),
				mcp.Min(0.0),
				mcp.Max(10.0),
			),
			mcp.WithString("product",
				mcp.Description("Product affected by the vulnerability"),
			),
			mcp.WithString("afterYear",
				mcp.Description("Filter CVEs published after the specified year"),
			),
			mcp.WithBool("chineseTranslationFirst",
				mcp.Description("Prioritize Chinese translation of the CVE description"),
			),
			mcp.WithString("keywords",
				mcp.Description("Keywords to search within the CVE description"),
			),
		), handleQueryCVE),
	)
}

func handleQueryCVE(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		cve := utils.MapGetString(args, "cve")
		if cve != "" {
			req := ypb.GetCVERequest{
				CVE: cve,
			}
			rsp, err := s.grpcClient.GetCVE(ctx, &req)
			if err != nil {
				return nil, utils.Wrap(err, "failed to query cve")
			}
			return NewCommonCallToolResult(rsp.CVE)
		} else {
			var req ypb.QueryCVERequest
			err := mapstructure.Decode(request.Params.Arguments, &req)
			if err != nil {
				return nil, utils.Wrap(err, "invalid argument")
			}
			rsp, err := s.grpcClient.QueryCVE(ctx, &req)
			if err != nil {
				return nil, utils.Wrap(err, "failed to query cve")
			}
			return NewCommonCallToolResult(rsp.Data)
		}
	}
}
