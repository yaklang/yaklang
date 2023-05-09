package yaktest

import (
	"github.com/yaklang/yaklang/common/consts"
	"testing"
)

func TestUpdateScript(t *testing.T) {
	consts.GetGormProjectDatabase()
	cases := []YakTestCase{
		{
			Name: "测试更新NaslScript",
			Src:  `nasl.UpdateDatabase("/Users/z3/nasl/nasl-plugins/2023/apache")`,
		},
	}
	Run("测试从本地文件更新NaslScript到数据库", t, cases...)
}

func TestInitNaslDatabase(t *testing.T) {
	consts.GetGormProjectDatabase()
	cases := []YakTestCase{
		{
			Name: "测试初始化NaslScript",
			Src: `
oracleGroupScriptPath=` + "`" + `
/Users/z3/nasl/nasl-plugins/2013/oracle
/Users/z3/nasl/nasl-plugins/2014/oracle
/Users/z3/nasl/nasl-plugins/2022/oracle
/Users/z3/nasl/nasl-plugins/2023/oracle
/Users/z3/nasl/nasl-plugins/2015/oracle
/Users/z3/nasl/nasl-plugins/2017/oracle
/Users/z3/nasl/nasl-plugins/2019/oracle
/Users/z3/nasl/nasl-plugins/2021/oracle
/Users/z3/nasl/nasl-plugins/2020/oracle
/Users/z3/nasl/nasl-plugins/2018/oracle
/Users/z3/nasl/nasl-plugins/2016/oracle
` + "`" + `
apacheGroupScriptPath=` + "`" + `
/Users/z3/nasl/nasl-plugins/2013/apache
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
/Users/z3/nasl/nasl-plugins/2016/apache
` + "`" + `
oracleGroupScriptName=[]
nasl.RemoveDatabase()
oracleGroupScriptPath.Split("\n").Map(func(path) {
	fileInfos = file.ReadFileInfoInDirectory(path)~
	fileInfos.Map(func(fileInfo) {
		if fileInfo.IsDir {
			return
		}
		if fileInfo.Path.HasSuffix(".nasl") {
			err = nasl.UpdateDatabase(fileInfo.Path)
			if err{
				log.Error(err)
				return
			}
			oracleGroupScriptName.Append(fileInfo.Name)	
		}
	})
})
dump(oracleGroupScriptName)
`,
		},
	}
	Run("测试初始化NaslScript到数据库", t, cases...)
}
