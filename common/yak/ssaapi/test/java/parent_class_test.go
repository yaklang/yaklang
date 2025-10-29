package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestReal_FromImplToInterface(t *testing.T) {

	vf := filesys.NewVirtualFs()
	{
		vf.AddFile("ruoyi-system/src/main/resources/mapper/system/SysDeptMapper.xml", `
	<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE mapper
PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN"
"http://mybatis.org/dtd/mybatis-3-mapper.dtd">
<mapper namespace="com.ruoyi.system.mapper.SysDeptMapper">
	<select id="selectDeptList" parameterType="SysDept" resultMap="SysDeptResult">
        <include refid="selectDeptVo"/>
        where d.del_flag = '0'
        <if test="parentId != null and parentId != 0">
			AND parent_id = #{parentId}
		</if>
		<if test="deptName != null and deptName != ''">
			AND dept_name like concat('%', #{deptName}, '%')
		</if>
		<if test="status != null and status != ''">
			AND status = #{status}
		</if>
		<!-- 数据范围过滤 -->
		${params.dataScope}
		order by d.parent_id, d.order_num
    </select>
</mapper> 
	`)
		vf.AddFile("ruoyi-system/src/main/java/com/ruoyi/system/mapper/SysDeptMapper.java", `
package com.ruoyi.system.mapper;

import java.util.List;
import com.ruoyi.system.domain.SysDept;

/**
 * 部门管理 数据层
 * 
 * @author ruoyi
 */
public interface SysDeptMapper
{

	/**
	* 查询部门管理数据
	* 
	* @param dept 部门信息
	* @return 部门信息集合
	*/
	public List<SysDept> selectDeptList(SysDept dept);

}
	`)

		vf.AddFile("ruoyi-system/src/main/java/com/ruoyi/system/service/ISysDeptService.java", `
	package com.ruoyi.system.service;

import java.util.List;
import com.ruoyi.system.domain.SysDept;

/**
 * 部门管理 服务层
 * 
 * @author ruoyi
 */
public interface ISysDeptService
{
    /**
     * 查询部门管理数据
     * 
     * @param dept 部门信息
     * @return 部门信息集合
     */
    public List<SysDept> selectDeptList(SysDept dept);
}
	`)

		vf.AddFile("ruoyi-system/src/main/java/com/ruoyi/system/service/impl/SysDeptServiceImpl.java", `
	package com.ruoyi.system.service.impl;

import java.util.List;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import com.ruoyi.common.annotation.DataScope;
import com.ruoyi.system.domain.SysDept;
import com.ruoyi.system.mapper.SysDeptMapper;
import com.ruoyi.system.service.ISysDeptService;

/**
 * 部门管理 服务实现
 * 
 * @author ruoyi
 */
@Service
public class SysDeptServiceImpl implements ISysDeptService
{
    @Autowired
    private SysDeptMapper deptMapper;

    /**
     * 查询部门管理数据
     * 
     * @param dept 部门信息
     * @return 部门信息集合
     */
    @Override
    @DataScope(deptAlias = "d")
    public List<SysDept> selectDeptList(SysDept dept)
    {
        return deptMapper.selectDeptList(dept);
    }
}
	`)

		vf.AddFile("ruoyi-admin/src/main/java/com/ruoyi/web/controller/system/SysDeptController.java", `
	package com.ruoyi.web.controller.system;

import java.util.List;
import org.apache.shiro.authz.annotation.RequiresPermissions;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Controller;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.ResponseBody;
import com.ruoyi.system.domain.SysDept;
import com.ruoyi.system.service.ISysDeptService;

/**
 * 部门信息
 * 
 * @author ruoyi
 */
@Controller
@RequestMapping("/system/dept")
public class SysDeptController extends BaseController
{

    @Autowired
    private ISysDeptService deptService;

    @RequiresPermissions("system:dept:list")
    @PostMapping("/list")
    @ResponseBody
    public List<SysDept> list(SysDept source)
    {
        List<SysDept> deptList = deptService.selectDeptList(source);
        return deptList;
    }
}
	`)
	}

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
<mybatisSink> as $Param
$Param #-> as $ParamTopDef
	`, map[string][]string{
		"ParamTopDef": {"Parameter-source"},
	}, true, ssaapi.WithLanguage(ssaconfig.JAVA),
	)
}

func TestInterfaceAndImpl(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/src/main/java/com/a/example/IA.java", `
package com.a.example;
class IA {
	public int get();
	public void set(int i);
}
	`)

	vf.AddFile("a/src/main/java/com/a/example/impl/IAImpl.java", `
	package com.a.example.impl;
	import com.a.example.IA;
	class IAImpl implements IA {
		public int get() {
			var target  = 11;
			return target;
		}
		public void set(int i) {
			var target1 = i;
		}
	}
	`)

	vf.AddFile("a/src/main/java/com/a/example/impl/IAImpl2.java", `
	package com.a.example.impl;
	import com.a.example.IA;
	class IAImpl2 implements IA {
		public int get() {
			return 22;
		}
		public void set(int i) {
			var target2 = i;
		}
	}
	`)

	vf.AddFile("a/src/main/java/com/a/example/User.java", `
package com.a.example;
import com.a.example.impl.IAImpl;
import com.a.example.impl.IAImpl2;
class User {
    private IA ia;
	private IAImpl iai; 
	private IAImpl2 iai2; 

	public void ff() {
		func0(ia.get()); // can get interface/impl1/impl2
		ia.set(0);

		func1(iai.get()); // impl1
		iai.set(1);
		func2(iai2.get()); // impl2
		iai2.set(2);
	}
}
	`)

	// pointer
	t.Run("test impl.function pointer with interface.function", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
	IAImpl.get as $func
	$func() as $call
	`, map[string][]string{
			"call": {"ia.get", "iai.get"},
		}, true, ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	// bottom user
	t.Run("test from impl to interface", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		target --> as $target
	`, map[string][]string{
			"target": {"func0", "func1"},
		}, true, ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	// from top to bottom
	t.Run("test from interface to impl", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		func0(* #-> as $interfaceTarget)
		func1(* #-> as $impl1Target)
		func2(* #-> as $impl2Target)
		`, map[string][]string{
			"interfaceTarget": {"11", "22"},
			"impl1Target":     {"11"},
			"impl2Target":     {"22"},
		}, true, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	// form bottom to top
	t.Run("test interface.function parameter top def", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		IA.set as $func 
		$func(* #-> ?{opcode: const} as $para) as $call
		`, map[string][]string{
			"para": {"0", "1", "2"},
		}, true, ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("test implement.function parameter top def", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
		IAImpl.set(* #-> ?{opcode: const} as $para1) as $call1
		target1 #-> as $target1

		IAImpl2.set(* #-> ?{opcode: const} as $para2) as $call2
		target2 #-> as $target1
		`, map[string][]string{
			"para1": {"1", "0"},
			"para2": {"2", "0"},
		}, true, ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})
}
func TestPackageDecl(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a.java", `package main;
class A{
	public B b;
	public void a(){
		println(b.a);
	}
}`)
	fs.AddFile("b.java", `package main;
class B{
	public static int a = 1;
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, fs, `println(* #-> * as $param)`,
		map[string][]string{},
		false,
		ssaapi.WithLanguage(ssaconfig.JAVA))
}
