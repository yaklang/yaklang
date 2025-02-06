package syntaxflow

import (
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func createTestVFS() filesys_interface.FileSystem {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("A.java", `package net.javaguides.usermanagement.web;

import java.io.IOException;
import java.sql.SQLException;
import java.util.List;

import javax.servlet.RequestDispatcher;
import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import net.javaguides.usermanagement.dao.UserDAO;
import net.javaguides.usermanagement.model.User;

/**
 * ControllerServlet.java
 * This servlet acts as a page controller for the application, handling all
 * requests from the user.
 * @email Ramesh Fadatare
 */

@WebServlet("/")
public class UserServlet extends HttpServlet {
	private static final long serialVersionUID = 1L;
	private UserDAO userDAO;
	
	public void init() {
		userDAO = new UserDAO();
	}

	protected void doPost(HttpServletRequest request, HttpServletResponse response)
			throws ServletException, IOException {
		doGet(request, response);
	}

	protected void doGet(HttpServletRequest request, HttpServletResponse response)
			throws ServletException, IOException {
		String action = request.getServletPath();

		try {
			switch (action) {
			case "/new":
				showNewForm(request, response);
				break;
			case "/insert":
				insertUser(request, response);
				break;
			case "/delete":
				deleteUser(request, response);
				break;
			case "/edit":
				showEditForm(request, response);
				break;
			case "/update":
				updateUser(request, response);
				break;
			default:
				listUser(request, response);
				break;
			}
		} catch (SQLException ex) {
			throw new ServletException(ex);
		}
	}

	private void listUser(HttpServletRequest request, HttpServletResponse response)
			throws SQLException, IOException, ServletException {
		List<User> listUser = userDAO.selectAllUsers();
		request.setAttribute("listUser", listUser);
		RequestDispatcher dispatcher = request.getRequestDispatcher("user-list.jsp");
		dispatcher.forward(request, response);
	}

	private void showNewForm(HttpServletRequest request, HttpServletResponse response)
			throws ServletException, IOException {
		RequestDispatcher dispatcher = request.getRequestDispatcher("user-form.jsp");
		dispatcher.forward(request, response);
	}

	private void showEditForm(HttpServletRequest request, HttpServletResponse response)
			throws SQLException, ServletException, IOException {
		int id = Integer.parseInt(request.getParameter("id"));
		User existingUser = userDAO.selectUser(id);
		RequestDispatcher dispatcher = request.getRequestDispatcher("user-form.jsp");
		request.setAttribute("user", existingUser);
		dispatcher.forward(request, response);

	}

	private void insertUser(HttpServletRequest request, HttpServletResponse response) 
			throws SQLException, IOException {
		String name = request.getParameter("name");
		String email = request.getParameter("email");
		String country = request.getParameter("country");
		User newUser = new User(name, email, country);
		userDAO.insertUser(newUser);
		response.sendRedirect("list");
	}

	private void updateUser(HttpServletRequest request, HttpServletResponse response) 
			throws SQLException, IOException {
		int id = Integer.parseInt(request.getParameter("id"));
		String name = request.getParameter("name");
		String email = request.getParameter("email");
		String country = request.getParameter("country");

		User book = new User(id, name, email, country);
		userDAO.updateUser(book);
		response.sendRedirect("list");
	}

	private void deleteUser(HttpServletRequest request, HttpServletResponse response) 
			throws SQLException, IOException {
		int id = Integer.parseInt(request.getParameter("id"));
		userDAO.deleteUser(id);
		response.sendRedirect("list");

	}

}`)
	return vfs
}

func TestLib_ServletParam(t *testing.T) {
	vfs := createTestVFS()
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		results, err := prog.SyntaxFlowWithError(`<include('java-servlet-param')> as $params`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		results.Show()
		require.Greater(t, len(results.GetValues("params")), 7)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

//go:embed syntaxflow_include_test.lib.sf
var sflib string

func TestSFLib(t *testing.T) {
	const ruleName = "fetch-abc-calling"
	_, err := sfdb.ImportRuleWithoutValid(ruleName, `
desc(lib: "abc");
abc() as $output;
alert $output
`, false)
	if err != nil {
		t.Fatal(err)
	}
	defer sfdb.DeleteRuleByRuleName(ruleName)

	ssatest.Check(t, `

abc = () => {
	return "abc"
}

e = d(abc())
dump(e)

`, func(prog *ssaapi.Program) error {
		results := prog.SyntaxFlowChain("<include(abc)> --> *").Show()
		if len(results) < 1 {
			t.Fatal("no result")
		}
		return nil
	})
}

func TestFS_RuleUpdate(t *testing.T) {
	name := "yak-a.sf"
	content := `
	desc(lib: "a")
	a as $a
	alert $a
	`
	sfdb.ImportRuleWithoutValid(name, content, true)
	defer sfdb.DeleteRuleByRuleName(name)

	ssatest.CheckSyntaxFlow(t, `
	a = 1 
	b = 2`,
		`
	<include(a)> as $target
	`, map[string][]string{
			"target": {"1"},
		},
	)

	// update
	content = `
	desc(lib: "b")
	b as $a
	alert $a
	`
	sfdb.ImportRuleWithoutValid(name, content, true)

	ssatest.CheckSyntaxFlow(t, `
	a = 1 
	b = 2`,
		`
	<include(b)> as $target
	`, map[string][]string{
			"target": {"2"},
		},
	)
}

func Test_Include_HitCache(t *testing.T) {
	programName := uuid.NewString()
	vfs := createTestVFS()
	prog, err := ssaapi.ParseProjectWithFS(vfs, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(ssaapi.JAVA))
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	require.NoError(t, err)
	require.NotNil(t, prog)

	ruleName := "java-servlet-param"
	prog.SyntaxFlowWithError(fmt.Sprintf(`<include('%s')>`, ruleName))

	cache := ssaapi.GetSFIncludeCache()
	require.Greater(t, cache.Count(), 0)
	cache.ForEach(func(s string, vo sfvm.ValueOperator) {
		t.Logf("key: %s, value: %v", s, vo)
	})
	_, exist := ssaapi.GetSFIncludeCache().Get(utils.CalcSha256(ruleName, programName))
	require.True(t, exist)
}

func TestSF_NativeCall_Include_Input_Value(t *testing.T) {
	code := `a1 = 1
	a2 ="hello world"
	b = 2`

	name := uuid.NewString()
	libName := uuid.NewString()
	content := fmt.Sprintf(`
		desc(lib: "%s");
		$input?{have:'hello world'} as $output;
		alert $output;
		`, libName)

	sfdb.ImportRuleWithoutValid(name, content, true)
	defer sfdb.DeleteRuleByRuleName(name)

	t.Run("test include have input param ", func(t *testing.T) {
		sfRule := fmt.Sprintf(`
		a* as $check;	
		$check<include('%s')> as $target`, libName)
		ssatest.CheckSyntaxFlow(t, code, sfRule, map[string][]string{
			"check":  {"1", "\"hello world\""},
			"target": {"\"hello world\""},
		})
	})

	t.Run("test cache hit", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("a.yak", code)

		sfdb.ImportRuleWithoutValid(name, content, true)
		defer sfdb.DeleteRuleByRuleName(name)

		sfRule := fmt.Sprintf(`
		a* as $check;	
		$check<include('%s')> as $target`, libName)

		programName := uuid.NewString()
		prog, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(consts.Yak))
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

		require.NoError(t, err)
		prog.SyntaxFlowWithError(sfRule)
		cache := ssaapi.GetSFIncludeCache()
		require.Greater(t, cache.Count(), 0)

		haveResult := false
		cache.ForEach(func(s string, vo sfvm.ValueOperator) {
			t.Logf("key: %s, value: %v", s, vo)
			if strings.Contains(vo.String(), "hello world") {
				haveResult = true
			}
		})
		require.True(t, haveResult)
	})

	t.Run("input value  should not affect program ", func(t *testing.T) {
		name2 := uuid.NewString()
		libName2 := uuid.NewString()
		content2 := fmt.Sprintf(`
		desc(lib: "%s");
		b as $output;
		alert $output;
		`, libName2)

		sfdb.ImportRuleWithoutValid(name2, content2, true)
		defer sfdb.DeleteRuleByRuleName(name2)

		sfRule := fmt.Sprintf(`
		a* as $check;	
		$check<include('%s')> as $target`, libName2)
		ssatest.CheckSyntaxFlow(t, code, sfRule, map[string][]string{
			"check":  {"1", "\"hello world\""},
			"target": {"2"},
		})
	})
}

func TestSF_Include_Cache_For_Recompile(t *testing.T) {
	programName := uuid.NewString()
	vfs := createTestVFS()
	prog1, err := ssaapi.ParseProjectWithFS(vfs, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithSaveToProfile(true))
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
	require.NoError(t, err)
	require.NotNil(t, prog1)

	ruleName := "java-servlet-param"
	prog1.SyntaxFlowWithError(fmt.Sprintf(`<include('%s')>`, ruleName))

	hash1, _, ok1 := ssaapi.GetIncludeCacheValue(prog1[0], ruleName, nil)
	require.True(t, ok1)

	// recompile
	progFromDB, err := ssaapi.FromDatabase(programName)
	require.NoError(t, err)
	hash2, _, ok2 := ssaapi.GetIncludeCacheValue(progFromDB, ruleName, nil)
	require.False(t, ok2)
	require.NotEqual(t, hash1, hash2)
}
