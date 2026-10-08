package scannode

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	cli "github.com/yaklang/yaklang/common/urfavecli"
)

// SSA IR schema DDL is available only through an explicit operations command;
// the ordinary node and distyak paths never invoke it for company databases.
var SSAIRSchemaCommand = cli.Command{
	Name:  "ssa-ir-schema",
	Usage: "Operations only: initialize, migrate, or check a company SSA IR database",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "action", Value: "check", Usage: "init, migrate, or check"},
		cli.StringFlag{Name: "company-id", Usage: "Company owning this database"},
		cli.StringFlag{Name: "dsn-env", Value: "LEGION_SSA_IR_OPS_DSN", Usage: "Environment variable containing the operations PostgreSQL DSN"},
	},
	Action: func(c *cli.Context) error {
		envName := strings.TrimSpace(c.String("dsn-env"))
		if envName == "" {
			return fmt.Errorf("DSN environment variable name is required")
		}
		raw := strings.TrimSpace(os.Getenv(envName))
		_ = os.Unsetenv(envName)
		if raw == "" {
			return fmt.Errorf("operations DSN environment variable is empty")
		}
		return consts.ManageCompanySSAIRSchema(raw, c.String("company-id"), c.String("action"))
	},
}

// ValidateNodeDatabaseEnvironment prevents AI/tools in the long-lived host from
// inheriting a company compiler connection. Only the isolated SSA task child
// receives that connection through its trusted dispatch environment.
func ValidateNodeDatabaseEnvironment() error {
	if strings.TrimSpace(os.Getenv(consts.ENV_SSA_DATABASE_RAW)) != "" || strings.TrimSpace(os.Getenv(consts.ENV_SSA_DATABASE_COMPANY_ID)) != "" {
		return fmt.Errorf("node-wide SSA database configuration is forbidden; use company task dispatch")
	}
	dialect, _ := consts.GetSSADataBaseInfo()
	if dialect != consts.SQLiteExtend && dialect != consts.SQLite {
		return fmt.Errorf("node-wide remote SSA database connection is forbidden")
	}
	return nil
}
