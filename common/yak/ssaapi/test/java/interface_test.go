package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestInterface(t *testing.T) {
	t.Run("test interface impl", func(t *testing.T) {
		code := `package com.example.demo1;


interface A {
    public int getA();
}

class B implements A {
    public int getA() {
		int target =1;
        return target;
    }
}

class C implements A {
    @Override
    public int getA() {
        return 0;
    }
}

public class test {
    public A a;
    public B b;
    public C c;

    public void testRun() {
        a.getA();
        b.getA();
        c.getA();
    }
}`
		ssatest.CheckSyntaxFlow(t, code, `target --> as $param`,
			map[string][]string{
				"param": {
					"Undefined-this.a.getA(valid)(ParameterMember-parameter[0].a)",
					"Undefined-this.b.getA(valid)(ParameterMember-parameter[0].b)",
				},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA))
	})
	t.Run("test big interface demo", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		/*
			-- mall-portal
				-- src/main/java/com/macro/mall/portal
					-- dao/HomeDao.java
					-- service/impl/PmsPortalBrandServiceImpl.java
		*/

		vf.AddFile("mall-portal/src/main/java/com/macro/mall/portal/dao/HomeDao.java", `
	package com.macro.mall.portal.dao;

import com.macro.mall.model.CmsSubject;
import com.macro.mall.model.PmsBrand;
import java.util.List;

public interface HomeDao {
    List<PmsBrand> getRecommendBrandList(@Param("offset") Integer off,@Param("limit") Integer limit);
}
	`)
		vf.AddFile("mall-portal/src/main/java/com/macro/mall/portal/service/impl/PmsPortalBrandServiceImpl.java", `
	package com.macro.mall.portal.service.impl;

import com.macro.mall.portal.dao.HomeDao;

@Service
public class PmsPortalBrandServiceImpl implements PmsPortalBrandService {
    @Autowired
    private HomeDao homeDao;

    @Override
    public List<PmsBrand> recommendList(Integer pageNum, Integer pageSize) {
        int offset = (pageNum - 1) * pageSize;
        return homeDao.getRecommendBrandList(offset, pageSize);
    }
}
	`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
	// function use-def
	HomeDao.getRecommendBrandList --> ?{opcode: call}  as $caller
	check $caller then "fine" else "not found interface method use-def"

	// parameter  use-def 
	off #-> as $ParameterDef
	check $ParameterDef then "fine" else "not found parameter offset defined"
`, map[string][]string{
			"caller":       {"Undefined-this.homeDao.getRecommendBrandList(valid)(ParameterMember-parameter[0].homeDao,mul(sub(Parameter-pageNum, 1), Parameter-pageSize),Parameter-pageSize)"},
			"ParameterDef": {"1", "Parameter-pageNum", "Parameter-pageSize"},
		},
			false,
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})
}
