package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestPythonSyntaxFlow_RequestAsHit_FlaskImport(t *testing.T) {
	code := `
from flask import Flask, render_template, request, session, redirect, url_for

app = Flask(__name__)

@app.route('/login', methods=['POST'])
def login():
    username = request.form.get('username')
    password = request.form.get('password')
    if username == '' and password == '':
        return render_template('index.html', msg='Fields Empty')
    return redirect(url_for('dashboard'))
`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`request as $hit`)
	require.NoError(t, err)

	hits := res.GetValues("hit")
	require.NotEmpty(t, hits, "SyntaxFlow rule `request as $hit` should resolve Flask `request` binding (TIWAP-style import)")
}

func TestPythonSyntaxFlow_AppRoute(t *testing.T) {
	code := `
from flask import Flask, request

app = Flask(__name__)

@app.route('/login', methods=['POST'])
def login():
    username = request.form.get('username')
    return username
`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`app.route(* as $route)`)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetValues("route"), "SyntaxFlow should match lowered @app.route(...) call")

	res, err = prog.SyntaxFlowWithError(`login?{opcode:function} as $handler`)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetValues("handler"), "decorated def login should remain a function value")
}

func TestPythonSyntaxFlow_AppRouteToHandler(t *testing.T) {
	code := `
from flask import Flask, request

app = Flask(__name__)

@app.route('/blind-sql-injection', methods=['POST', 'GET'])
@is_logged
def blind_sql_injection():
    return request.data
`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`app.route(*, * as $handler, * as $path)`)
	require.NoError(t, err)
	handlers := res.GetValues("handler")
	require.NotEmpty(t, handlers, "second app.route arg should be the handler function")
	for _, v := range handlers {
		require.Contains(t, v.String(), "blind_sql_injection")
	}
	paths := res.GetValues("path")
	require.NotEmpty(t, paths)
	foundPath := false
	for _, v := range paths {
		if strings.Contains(v.String(), "blind-sql-injection") {
			foundPath = true
			break
		}
	}
	require.True(t, foundPath, "route path arg should contain /blind-sql-injection")
}
