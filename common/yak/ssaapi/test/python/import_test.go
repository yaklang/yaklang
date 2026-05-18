package python

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPython_ImportWithInit(t *testing.T) {
	t.Run("import with init", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("__init__.py", "")
		vf.AddFile("db_manager.py", `
# Imports
import sqlite3
from hashlib import md5

class DBManager:
    def __init__(self):
        # Initialize Database
        self.conn = sqlite3.connect('TIWAP.db', check_same_thread=False)
        self.cur = self.conn.cursor()

    def get_db_connection(self):
        return self.conn
`)
		vf.AddFile("sqli_app.py", `
from db_manager import DBManager
from hashlib import md5
import sqlite3

# SQL Injection - Low
def sqli_low(username, password):
    dbmanager = DBManager()

    cur = dbmanager.get_db_connection().cursor()

    if dbmanager.check_user(username=username):
        return "User Exists"

    try:
        stmt = "SELECT userid, username FROM users WHERE username='%s' AND password='%s'" \
               % (str(username), str(password))

        result = cur.execute(stmt)

    except sqlite3.OperationalError as e:
        return e

    return result.fetchall()
`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
	sqlite3.connect as $connect
	*.cursor?(* <slice(index=0)> #{until: "* & $connect"}-> ) as $func
`, map[string][]string{
			"connect": {"Undefined-sqlite3.connect(valid)"},
			"func":    {`Undefined-self.conn.cursor(Undefined-sqlite3.connect(valid)("TIWAP.db",false))`},
		}, true, ssaapi.WithLanguage(ssaconfig.PYTHON))
	})

	t.Run("import with init and global", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("helper/__init__.py", "")
		vf.AddFile("helper/db_manager.py", `
# Imports
import sqlite3
from hashlib import md5

class DBManager:
    def __init__(self):
        # Initialize Database
        self.conn = sqlite3.connect('TIWAP.db', check_same_thread=False)
        self.cur = self.conn.cursor()

    def get_db_connection(self):
        return self.conn

    def close_db_connection(self):
        return self.cur.close()

    def commit_db(self):
        return self.conn.commit()

    def check_user(self, username):
        self.create_db_connection()
        result = self.cur.execute("SELECT username FROM users WHERE username = ?", (username,))

        if type(result) != 'NoneType':
            if self.cur.fetchone() is not None:
                self.close_db_connection()
                return True

        self.close_db_connection()
        return False
`)
		vf.AddFile("sqli_app.py", `
from helper.db_manager import DBManager
from hashlib import md5
import sqlite3

dbmanager = DBManager()

# SQL Injection - Low
def sqli_low(username, password):
    global dbmanager
    cur = dbmanager.get_db_connection().cursor()

    if dbmanager.check_user(username=username):
        return "User Exists"

    try:
        stmt = "SELECT userid, username FROM users WHERE username='%s' AND password='%s'" \
               % (str(username), str(password))

        result = cur.execute(stmt)

    except sqlite3.OperationalError as e:
        return e

    return result.fetchall()
`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
sqlite3.connect as $connect
*.cursor?(* <slice(index=0)> #{until: "* & $connect"}-> ) as $func
`, map[string][]string{
			"connect": {"Undefined-sqlite3.connect(valid)"},
			"func":    {`Undefined-self.conn.cursor(Undefined-sqlite3.connect(valid)("TIWAP.db",false))`},
		}, true, ssaapi.WithLanguage(ssaconfig.PYTHON))
	})

}

// TestPython_ImportSubModule_CalleeResolvesToFunction ensures bare and member calls to
// cmd_injection_low both resolve to Function-cmd_injection_low (not FreeValue / Undefined).
func TestPython_ImportSubModule_CalleeResolvesToFunction(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("vulnerabilities/CommandInjection.py", `
def cmd_injection_low(query):
    return query

def cmd_injection_medium(query):
    return cmd_injection_low(query)
`)
	vf.AddFile("app.py", `
from vulnerabilities import CommandInjection

def cmd_injection_route(query):
    ci = CommandInjection
    return ci.cmd_injection_low(query=query)
`)

	ssatest.CheckResultWithFS(t, vf, `cmd_injection_low as $callee`, func(res *ssaapi.SyntaxFlowResult) {
		vals := res.GetValues("callee")
		res.Show()
		require.NotEmpty(t, vals)
		for _, v := range vals {
			s := v.String()
			require.True(t, strings.HasPrefix(s, "Function-cmd_injection_low"), "got %s", s)
			require.NotContains(t, s, "FreeValue")
			require.NotContains(t, s, "Undefined")
		}
	}, ssaapi.WithLanguage(ssaconfig.PYTHON))
}
