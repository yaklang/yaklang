package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSharp_RealWorld_ASPNETRequestToSQLCommand(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using Microsoft.AspNetCore.Mvc;
using System.Data.SqlClient;

namespace Demo.Controllers {
    public class UsersController : ControllerBase {
        public object Get() {
            var id = Request.Query["id"];
            var sql = "select * from users where id=" + id;
            var command = new SqlCommand(sql);
            return command.ExecuteScalar();
        }
    }
}
`)
	result, err := prog.SyntaxFlowWithError(`SqlCommand(* #-> as $input)`)
	require.NoError(t, err)
	inputs := result.GetValues("input")
	require.NotEmpty(t, inputs, "SQL constructor arguments must remain queryable")
	require.True(t, strings.Contains(inputs.String(), "Request") || strings.Contains(inputs.String(), "id"),
		"request-derived value must flow into SqlCommand: %s", inputs.String())
}
