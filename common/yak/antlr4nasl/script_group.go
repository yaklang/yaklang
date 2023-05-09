package antlr4nasl

import "strings"

func InitPluginGroup(engine *ScriptEngine) {
	apachePath := `/Users/z3/nasl/nasl-plugins/2013/apache
/Users/z3/nasl/nasl-plugins/2014/apache
/Users/z3/nasl/nasl-plugins/2022/apache
/Users/z3/nasl/nasl-plugins/2023/apache
/Users/z3/nasl/nasl-plugins/2012/apache
/Users/z3/nasl/nasl-plugins/2009/apache
/Users/z3/nasl/nasl-plugins/2017/apache
/Users/z3/nasl/nasl-plugins/2010/apache
/Users/z3/nasl/nasl-plugins/2019/apache
/Users/z3/nasl/nasl-plugins/2021/apache
/Users/z3/nasl/nasl-plugins/2020/apache
/Users/z3/nasl/nasl-plugins/2018/apache
/Users/z3/nasl/nasl-plugins/2011/apache
/Users/z3/nasl/nasl-plugins/2016/apache`
	oraclePath := `/Users/z3/nasl/nasl-plugins/2013/oracle
/Users/z3/nasl/nasl-plugins/2014/oracle
/Users/z3/nasl/nasl-plugins/2022/oracle
/Users/z3/nasl/nasl-plugins/2023/oracle
/Users/z3/nasl/nasl-plugins/2015/oracle
/Users/z3/nasl/nasl-plugins/2017/oracle
/Users/z3/nasl/nasl-plugins/2019/oracle
/Users/z3/nasl/nasl-plugins/2021/oracle
/Users/z3/nasl/nasl-plugins/2020/oracle
/Users/z3/nasl/nasl-plugins/2018/oracle
/Users/z3/nasl/nasl-plugins/2016/oracle`
	for _, path := range strings.Split(apachePath, "\n") {
		engine.AddScriptIntoGroup(PluginGroupApache, path)
	}
	for _, path := range strings.Split(oraclePath, "\n") {
		engine.AddScriptIntoGroup(PluginGroupOracle, path)
	}
}
