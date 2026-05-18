package test

import (
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
