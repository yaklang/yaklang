package java

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestInterface(t *testing.T) {
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
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}
