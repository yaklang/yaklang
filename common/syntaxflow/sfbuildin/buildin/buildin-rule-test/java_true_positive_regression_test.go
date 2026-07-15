package buildin_rule

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// This file is the long-term home for *true-positive* (and paired negative)
// regression tests of the builtin Java SyntaxFlow rules. It is table-driven:
// every time a rule's sink/source coverage is extended, append a row here
// instead of opening a new test file.
//
// Rules are referenced by their embed-FS path via sfbuildin.GetEmbedRuleContent,
// NOT by embedding a rule copy. Embedding a copy causes "drift": the test stays
// green after the real rule is changed because it runs the stale copy. Referencing
// the builtin by path keeps the test pinned to the rule users actually run.
//
// The SSA-engine feature tests (prog.SyntaxFlowChain(...).Len(), SSA API
// behaviour, etc.) live under common/yak/ssaapi/test/java/; they are a
// different concern and should NOT host builtin-rule regression tests.

// alertCounts tallies SyntaxFlow alert values by severity bucket so a single
// table row can assert either "must report high" (true positive) or "must
// report nothing" (negative/safe sample).
type alertCounts struct {
	Total int
	High  int
	Mid   int
	Low   int
}

// loadBuiltinRule reads a builtin rule straight from the embed FS by its
// relative path under buildin/ (e.g. "java/cwe-78-.../rule.sf"). No DB sync,
// no rule copy — the test always runs the rule that ships in the binary.
func loadBuiltinRule(t *testing.T, relativePath string) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent(relativePath)
	if !ok {
		t.Fatalf("builtin rule not found in embed fs: %s", relativePath)
	}
	require.NotEmpty(t, content, "builtin rule content should not be empty: %s", relativePath)
	return content
}

// runJavaBuiltinRule parses a single virtual Java file and runs the given rule
// content against it, returning alert counts grouped by severity.
func runJavaBuiltinRule(t *testing.T, ruleContent, filename, code string) alertCounts {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)

	programs, err := ssaapi.ParseProjectWithFS(vfs, ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	require.NotEmpty(t, programs)

	result, err := programs[0].SyntaxFlowWithError(ruleContent)
	require.NoError(t, err)

	c := alertCounts{}
	for _, variable := range result.GetAlertVariables() {
		n := len(result.GetValues(variable))
		c.Total += n
		info, ok := result.GetAlertInfo(variable)
		if !ok || info == nil {
			continue
		}
		switch info.Severity {
		case "critical", "high", "h":
			c.High += n
		case "middle", "medium", "mid", "m":
			c.Mid += n
		case "low", "info", "warning", "warn", "w":
			c.Low += n
		}
	}
	return c
}

// TestJavaTruePositiveRegressionRules is the append-friendly true-positive /
// negative regression table for builtin Java rules.
//
// Row semantics:
//   - wantHigh > 0       : vulnerable sample must produce at least this many
//                          high severity alerts (true positive).
//   - negative == true   : safe sample must produce NO alert at all.
//   - allowZeroHigh == true: the sample is a KNOWN-UNCOVERED gap (the rule does
//                            not yet reach this sink). It must not be reported
//                            as high today; when the rule is extended to cover
//                            it, flip the row to a positive wantHigh so the
//                            coverage is locked in. Keeps the gap visible.
//
// Add one positive row + one paired negative row whenever a sink/source is
// extended, so broader coverage cannot silently flag safe code.
func TestJavaTruePositiveRegressionRules(t *testing.T) {
	const cmdiRule  = "java/cwe-78-os-command-injection/java-servlet-n-spring-direct-command-injection.sf"
	const sqliRule  = "java/cwe-89-sql-injection/java-execute-query-string-add-out-of-control.sf"
	const xssRule   = "java/cwe-79-xss/java-servlet-xss.sf"
	const ldapRule  = "java/cwe-90-ldap-injection/java-ldap-injection.sf"
	const xpathRule = "java/cwe-643-xpath-injection/java-direct-xpath-injection.sf"

	cases := []struct {
		name          string
		rulePath      string
		fileName      string
		code          string
		wantHigh      int  // >0: expect >= this many high alerts (true positive)
		wantMid       int  // >0: expect >= this many mid alerts (for mid-level rules)
		negative      bool // true: safe sample, expect zero alerts total
		allowZeroHigh bool // true: known-uncovered gap, must not be high today
	}{
		// ---------------- CWE-78: OS Command Injection ----------------
		{
			name:     "cmdi_runtime_exec_tainted_is_detected",
			rulePath: cmdiRule,
			fileName: "CmdiRuntimeExec.java",
			wantHigh: 1,
			code: `
package securibench.micro.basic;

import javax.servlet.http.*;
import java.io.IOException;

@WebServlet("/cmdi")
public class CmdiRuntimeExec extends HttpServlet {
    protected void doGet(HttpServletRequest request, HttpServletResponse response) throws IOException {
        String cmd = request.getParameter("cmd");
        Runtime.getRuntime().exec(cmd);
    }
}`,
		},
		{
			name:     "cmdi_constant_command_not_reported",
			rulePath: cmdiRule,
			fileName: "SafeConstantExec.java",
			negative: true,
			code: `
package securibench.micro.basic;

import java.io.IOException;

public class SafeConstantExec {
    public void run() throws IOException {
        Runtime.getRuntime().exec("ls -la");
    }
}`,
		},
		// ---------------- CWE-89: SQL Injection ----------------
		// executeUpdate (3 overloads) + executeQuery (1): the rule's glob
		// `.createStatement().execute*(,* as $params)` must cover all four
		// sinks. The rule deduplicates tainted sink reaches per alert
		// variable, so a single program with several tainted sinks still
		// yields one high alert (verified: an executeUpdate-only sample also
		// yields one high alert). Assert wantHigh:1 — the point is that the
		// executeUpdate sinks are reached at all, not that each emits its
		// own alert.
		{
			name:     "sqli_execute_update_and_execute_query_sinks",
			rulePath: sqliRule,
			fileName: "Basic21.java",
			wantHigh: 1,
			code: `
package securibench.micro.basic;

import java.io.IOException;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Locale;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import securibench.micro.BasicTestCase;
import securibench.micro.MicroTestCase;

public class Basic21 extends BasicTestCase implements MicroTestCase {
    private static final String FIELD_NAME = "name";

    protected void doGet(HttpServletRequest req, HttpServletResponse resp) throws IOException {
        String s = req.getParameter(FIELD_NAME);
        String name = s.toLowerCase(Locale.UK);

        Connection con = null;
        try {
            con = DriverManager.getConnection(MicroTestCase.CONNECTION_STRING);
            Statement stmt = con.createStatement();
            stmt.executeUpdate("select * from Users where name=" + name);
            stmt.executeUpdate("select * from Users where name=" + name, 0);
            stmt.executeUpdate("select * from Users where name=" + name,
                new String[] {});
            stmt.executeQuery("select * from Users where name=" + name);
        } catch (SQLException e) {
            System.err.println("An error occurred");
        } finally {
            try {
                if (con != null) con.close();
            } catch (SQLException e) {
                e.printStackTrace();
            }
        }
    }
}`,
		},
		// Statement.execute with tainted concatenation is a third sink the glob
		// must cover (distinct from executeQuery / executeUpdate).
		{
			name:     "sqli_statement_execute_sink",
			rulePath: sqliRule,
			fileName: "Basic20.java",
			wantHigh: 1,
			code: `
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
            stmt.execute("select * from Users where name=" + name);
        } catch (SQLException e) {
            System.err.println("An error occurred");
        } finally {
            try { if (con != null) con.close(); } catch (SQLException e) {}
        }
    }
}`,
		},
		// Safe parameterized query (placeholder + setString, no concatenation)
		// must NOT be reported — guards the expanded sink coverage against FPs.
		{
			name:     "sqli_safe_prepared_statement_not_reported",
			rulePath: sqliRule,
			fileName: "SafePreparedStatement.java",
			negative: true,
			code: `
package securibench.micro.basic;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import javax.servlet.http.HttpServletRequest;

public class SafePreparedStatement {
    public void doGet(HttpServletRequest req, Connection con) throws Exception {
        String userId = req.getParameter("id");
        String sql = "SELECT * FROM users WHERE id = ?";
        PreparedStatement pstmt = con.prepareStatement(sql);
        pstmt.setString(1, userId);
        ResultSet rs = pstmt.executeQuery();
    }
}`,
		},
		// prepareStatement with tainted concatenation: the SQL string argument
		// is built by concatenating user input, so prepareStatement is a SQLi
		// sink. The rule matches prepareStatement by method name; the taint
		// propagation only alerts when the SQL argument itself is tainted, so
		// the safe prepared statement above (placeholder + setString) is not
		// flagged. Paired with the sqli_safe_prepared_statement_not_reported
		// negative row.
		{
			name:     "sqli_prepare_statement_concat_detected",
			rulePath: sqliRule,
			fileName:  "Basic19.java",
			wantHigh:  1,
			code: `
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
            con.prepareStatement("select * from Users where name=" + name);
        } catch (SQLException e) {
            System.err.println("An error occurred");
        } finally {
            try { if (con != null) con.close(); } catch (SQLException e) {}
        }
    }
		}`,
		},
		// ---------------- CWE-79: Cross-Site Scripting (Servlet) ----------------
		// The sink matcher uses <getCallee><typeName> to filter by
		// HttpServletResponse type rather than receiver variable name
		// (which is arbitrary and renamed to parameter[N] by SSA).
		{
			name:     "xss_servlet_getWriter_println_tainted_is_detected",
			rulePath: xssRule,
			fileName: "Basic1.java",
			wantMid:  1,
			code: `
package securibench.micro.basic;
import java.io.IOException;
import java.io.PrintWriter;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import securibench.micro.BasicTestCase;
import securibench.micro.MicroTestCase;

public class Basic1 extends BasicTestCase implements MicroTestCase {
    protected void doGet(HttpServletRequest req, HttpServletResponse resp) throws IOException {
        String str = req.getParameter("name");
        PrintWriter writer = resp.getWriter();
        writer.println(str);    /* BAD */
    }
}`,
		},
		// Paired negative: a custom class with a getWriter() method that is
		// NOT HttpServletResponse must not be flagged by the type filter.
		{
			name:     "xss_non_http_response_getWriter_not_reported",
			rulePath: xssRule,
			fileName: "FakeWriter.java",
			negative: true,
			code: `
package test.fake;
import java.io.IOException;
import java.io.PrintWriter;
import javax.servlet.http.HttpServletRequest;

public class FakeWriter {
    private PrintWriter internalWriter;
    public PrintWriter getWriter() { return internalWriter; }
    public void doGet(HttpServletRequest req) throws IOException {
        String str = req.getParameter("name");
        FakeWriter fw = new FakeWriter();
        PrintWriter w = fw.getWriter();
        w.println(str);
    }
}`,
		},
		// ---------------- CWE-90: LDAP Injection ----------------
		{
			name:     "ldap_dircontext_search_tainted_is_detected",
			rulePath: ldapRule,
			fileName: "LdapSearch.java",
			wantHigh: 1,
			code: `
package org.owasp.benchmark.testcode;

import java.io.IOException;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

@WebServlet(value = "/ldapi-00/LdapSearch")
public class LdapSearch extends HttpServlet {
    private static final long serialVersionUID = 1L;

    @Override
    public void doGet(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        response.setContentType("text/html;charset=UTF-8");

        String param = "";
        java.util.Enumeration<String> headers = request.getHeaders("LdapSearch");
        if (headers != null && headers.hasMoreElements()) {
            param = headers.nextElement();
        }
        param = java.net.URLDecoder.decode(param, "UTF-8");

        try {
            String base = "ou=users,ou=system";
            javax.naming.directory.SearchControls sc = new javax.naming.directory.SearchControls();
            sc.setSearchScope(javax.naming.directory.SearchControls.SUBTREE_SCOPE);
            String filter = "(&(objectclass=person))(|(uid=" + param + ")(street={0}))";
            Object[] filters = new Object[] {"The streetz 4 Ms bar"};
            javax.naming.directory.DirContext ctx = new javax.naming.directory.InitialDirContext();
            javax.naming.NamingEnumeration<javax.naming.directory.SearchResult> results =
                    ctx.search(base, filter, filters, sc);
        } catch (Exception e) {
            System.out.println("LDAP search failed");
        }
    }
}`,
		},
		{
			// Known precision gap: parameterized LDAP search with {0} placeholders
			// still triggers because taint flows through Object[] filters into
			// .search(). The rule cannot distinguish {0} placeholders from string
			// concatenation at the SyntaxFlow level. Documented as a known FP;
			// remove this row when the rule gains Object[] arg sanitizer logic.
			name:          "ldap_parameterized_search_known_fp_gap",
			rulePath:      ldapRule,
			fileName:      "SafeLdapSearch.java",
			allowZeroHigh: false,
			wantHigh:      1, // intentionally expect the FP to be reported; flip to negative when fixed
			code: `
package org.owasp.benchmark.testcode;

import java.io.IOException;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

@WebServlet(value = "/ldapi-00/SafeLdapSearch")
public class SafeLdapSearch extends HttpServlet {
    private static final long serialVersionUID = 1L;

    @Override
    public void doGet(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        response.setContentType("text/html;charset=UTF-8");

        String username = request.getParameter("username");

        try {
            String base = "ou=users,ou=system";
            javax.naming.directory.SearchControls sc = new javax.naming.directory.SearchControls();
            sc.setSearchScope(javax.naming.directory.SearchControls.SUBTREE_SCOPE);
            String filter = "(&(objectclass=person)(uid={0}))";
            Object[] filters = new Object[] {username};
            javax.naming.directory.DirContext ctx = new javax.naming.directory.InitialDirContext();
            javax.naming.NamingEnumeration<javax.naming.directory.SearchResult> results =
                    ctx.search(base, filter, filters, sc);
        } catch (Exception e) {
            System.out.println("LDAP search failed");
        }
    }
}`,
		},
		// ---------------- CWE-643: XPath Injection ----------------
		{
			name:     "xpath_compile_tainted_is_detected",
			rulePath: xpathRule,
			fileName: "XPathCompile.java",
			wantHigh: 1,
			code: `
package org.owasp.benchmark.testcode;

import java.io.IOException;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

@WebServlet(value = "/xpathi-00/XPathCompile")
public class XPathCompile extends HttpServlet {
    private static final long serialVersionUID = 1L;

    @Override
    public void doGet(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        response.setContentType("text/html;charset=UTF-8");

        String param = request.getParameter("userInput");

        try {
            javax.xml.xpath.XPathFactory xpf = javax.xml.xpath.XPathFactory.newInstance();
            javax.xml.xpath.XPath xp = xpf.newXPath();
            String expression = "/Employees/Employee[@emplid='" + param + "']";
            org.w3c.dom.NodeList nodeList =
                    (org.w3c.dom.NodeList)
                            xp.compile(expression)
                                    .evaluate(new org.w3c.dom.Document() {}, javax.xml.xpath.XPathConstants.NODESET);
        } catch (Exception e) {
            System.out.println("XPath query failed");
        }
    }
}`,
		},
		{
			name:     "xpath_static_expression_not_reported",
			rulePath: xpathRule,
			fileName: "SafeXPath.java",
			negative: true,
			code: `
package org.owasp.benchmark.testcode;

import java.io.IOException;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

@WebServlet(value = "/xpathi-00/SafeXPath")
public class SafeXPath extends HttpServlet {
    private static final long serialVersionUID = 1L;

    @Override
    public void doGet(HttpServletRequest request, HttpServletResponse response)
            throws ServletException, IOException {
        response.setContentType("text/html;charset=UTF-8");

        try {
            javax.xml.xpath.XPathFactory xpf = javax.xml.xpath.XPathFactory.newInstance();
            javax.xml.xpath.XPath xp = xpf.newXPath();
            String expression = "/Employees/Employee[@emplid='12345']";
            org.w3c.dom.NodeList nodeList =
                    (org.w3c.dom.NodeList)
                            xp.evaluate(expression, new org.w3c.dom.Document() {}, javax.xml.xpath.XPathConstants.NODESET);
        } catch (Exception e) {
            System.out.println("XPath query failed");
        }
    }
}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rule := loadBuiltinRule(t, c.rulePath)
			counts := runJavaBuiltinRule(t, rule, c.fileName, c.code)

			switch {
			case c.allowZeroHigh:
				require.Zero(t, counts.High, "known gap must not be reported as high yet")
				t.Logf("prepareStatement gap: total=%d high=%d mid=%d low=%d (flip to wantHigh when covered)",
					counts.Total, counts.High, counts.Mid, counts.Low)
			case c.negative:
				require.Zero(t, counts.Total, "safe/negative sample must produce no alerts")
			case c.wantMid > 0:
				require.GreaterOrEqual(t, counts.Mid, c.wantMid,
					"vulnerable sample must produce at least %d mid alert(s)", c.wantMid)
			default:
				require.GreaterOrEqual(t, counts.High, c.wantHigh,
					"vulnerable sample must produce at least %d high alert(s)", c.wantHigh)
			}
		})
	}
}