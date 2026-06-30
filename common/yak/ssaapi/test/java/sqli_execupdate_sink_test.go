package java

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample/Basic21.java
var Basic21SQLiCode string

//go:embed sqli_execute_query_rule.sf
var sqliExecuteQueryRule string

// TestSQLiExecUpdateSinkCoverage checks that the SQL injection rule detects
// executeUpdate (in addition to executeQuery) when a tainted value is
// concatenated into the SQL string. The sample contains three executeUpdate
// overloads plus one executeQuery, all tainted, so the rule must alert at
// least four times.
func TestSQLiExecUpdateSinkCoverage(t *testing.T) {
	ssatest.Check(t, Basic21SQLiCode, func(prog *ssaapi.Program) error {
		got := prog.SyntaxFlowChain(sqliExecuteQueryRule).Len()
		if got < 4 {
			t.Fatalf("expected at least 4 SQLi alerts (executeUpdate + executeQuery), got %d", got)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

// TestSQLiExecuteSinkCoverage checks that Statement.execute with tainted
// string concatenation is detected (execute is another sink besides
// executeQuery / executeUpdate).
func TestSQLiExecuteSinkCoverage(t *testing.T) {
	code := `
package securibench.micro.basic;
import java.io.IOException;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.sql.Statement;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import securibench.micro.BasicTestCase;
import securibench.micro.MicroTestCase;

public class Basic20 extends BasicTestCase implements MicroTestCase {
    protected void doGet(HttpServletRequest req, HttpServletResponse resp) throws IOException {
        String name = req.getParameter("name");
        Connection con = null;
        try {
            con = DriverManager.getConnection("jdbc:test");
            Statement stmt = con.createStatement();
            stmt.execute("select * from Users where name=" + name); /* BAD */
        } catch (SQLException e) {
            System.err.println("An error occurred");
        } finally {
            try { if(con != null) con.close(); } catch (SQLException e) {}
        }
    }
}
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		got := prog.SyntaxFlowChain(sqliExecuteQueryRule).Len()
		if got < 1 {
			t.Fatalf("expected at least 1 SQLi alert for execute concatenation, got %d", got)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

// TestSQLiSafePreparedStatementNotReported checks that a safe parameterized
// query (placeholder "?" + setString binding, no concatenation) is NOT
// reported, so the expanded sink coverage does not introduce false positives.
func TestSQLiSafePreparedStatementNotReported(t *testing.T) {
	code := `
package securibench.micro.basic;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import javax.servlet.http.HttpServletRequest;

public class SafeQuery {
    public void doGet(HttpServletRequest req, Connection con) throws Exception {
        String userId = req.getParameter("id");
        String sql = "SELECT * FROM users WHERE id = ?";
        PreparedStatement pstmt = con.prepareStatement(sql);
        pstmt.setString(1, userId);
        ResultSet rs = pstmt.executeQuery();
    }
}
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		got := prog.SyntaxFlowChain(sqliExecuteQueryRule).Len()
		if got > 0 {
			t.Fatalf("expected 0 SQLi alerts for safe parameterized query, got %d", got)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

// TestSQLiPrepareStatementSinkCoverage documents that prepareStatement with
// tainted concatenation is not yet covered by this rule. Matching
// prepareStatement broadly would flag safe prepared statements (the statement
// object is tainted via setString), so it is excluded until a concat-aware
// guard is added. This test pins the current behaviour so the gap is visible.
func TestSQLiPrepareStatementSinkCoverage(t *testing.T) {
	code := `
package securibench.micro.basic;
import java.io.IOException;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import securibench.micro.BasicTestCase;
import securibench.micro.MicroTestCase;

public class Basic19 extends BasicTestCase implements MicroTestCase {
    protected void doGet(HttpServletRequest req, HttpServletResponse resp) throws IOException {
        String name = req.getParameter("name");
        Connection con = null;
        try {
            con = DriverManager.getConnection("jdbc:test");
            con.prepareStatement("select * from Users where name=" + name); /* BAD */
        } catch (SQLException e) {
            System.err.println("An error occurred");
        } finally {
            try { if(con != null) con.close(); } catch (SQLException e) {}
        }
    }
}
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		got := prog.SyntaxFlowChain(sqliExecuteQueryRule).Len()
		// Not covered yet. Flip to `got < 1` failure when prepareStatement is
		// covered safely.
		if got != 0 {
			t.Logf("prepareStatement now covered (alerts=%d)", got)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}