package java

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample/springboot
var springbootLoader embed.FS

func TestExtraFileAnalyzer(t *testing.T) {
	prog, err := ssaapi.ParseProjectWithFS(
		filesys.NewEmbedFS(springbootLoader),
		ssaapi.WithLanguage(ssaconfig.JAVA),
	)
	if err != nil {
		t.Fatal(err)
	}
	res, err := prog.SyntaxFlowWithError(
		`${application.properties}.re(/url=(.*)/) as $url`,
	)
	assert.NoErrorf(t, err, "SyntaxFlowWithError error: %v", err)
	res.Show()
	assert.Greater(t, res.GetValues("url").Len(), 0)
}

func TestSimpleExtraFile(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/resources/application.properties", `
	spring.datasource.url=jdbc:mysql://localhost:3306/your_database
	spring.datasource.username=your_username
	spring.datasource.password=your_password
	spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
	spring.jpa.hibernate.ddl-auto=update
	spring.jpa.properties.hibernate.dialect=org.hibernate.dialect.MySQL5InnoDBDialect
	`)
	vf.AddFile("src/resources/mapper/AMapper.xml", `
	<?xml version="1.0" encoding="UTF-8" ?>
	<!DOCTYPE mapper
	PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN"
	"http://mybatis.org/dtd/mybatis-3-mapper.dtd">
	<mapper namespace="com.mapper.UserMapper">

	<select id="selectUserByUsername" parameterType="String" resultType="com.po.User">
		select * from user where username=${username}
	</select>

	<insert id="insertUsetByUsername" parameterType="com.po.User">
		insert into user(username,userpasswd)values(#{username},#{userpasswd});
	</insert>

	</mapper>
	`)

	t.Run("test simple config file", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf,
			`${*.properties}.re(/spring.datasource.url=(.*)/) as $url`,
			map[string][]string{
				"url": {
					`"spring.datasource.url=jdbc:mysql://localhost:3306/your_database"`,
					`"jdbc:mysql://localhost:3306/your_database"`,
				},
			}, false,
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("test simple mybatis mapper file", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf,
			`
		${*Mapper.xml}.xpath("//mapper/*[contains(., '${') and @id]/@id") as $url
		${*Mapper.xml}.xpath("string(//mapper/*[contains(., '${') and @id]/@id)") as $url2
		`,
			map[string][]string{
				"url":  {`"selectUserByUsername"`},
				"url2": {`"selectUserByUsername"`},
			}, false,
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})
}

func TestExtraFile_GetFunction(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/resources/dao/HomeDao.xml", `
	<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE mapper PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN" "http://mybatis.org/dtd/mybatis-3-mapper.dtd">
<mapper namespace="com.macro.mall.portal.dao.HomeDao">
    <resultMap id="flashPromotionProduct" type="com.macro.mall.portal.domain.FlashPromotionProduct"
               extends="com.macro.mall.mapper.PmsProductMapper.BaseResultMap">
        <result column="flash_promotion_price" property="flashPromotionPrice"/>
        <result column="flash_promotion_count" property="flashPromotionCount"/>
        <result column="flash_promotion_limit" property="flashPromotionLimit"/>
    </resultMap>

    <select id="getRecommendBrandList" resultMap="com.macro.mall.mapper.PmsBrandMapper.BaseResultMap">
        SELECT b.*
        FROM
            sms_home_brand hb
            LEFT JOIN pms_brand b ON hb.brand_id = b.id
        WHERE
            hb.recommend_status = 1
            AND b.show_status = 1
        ORDER BY
            hb.sort DESC
        LIMIT #{offset}, #{limit}
    </select>

    <select id="getFlashProductList" resultMap="flashPromotionProduct">
        SELECT
            pr.flash_promotion_price,
            pr.flash_promotion_count,
            pr.flash_promotion_limit,
            p.*
        FROM
            sms_flash_promotion_product_relation pr
            LEFT JOIN pms_product p ON pr.product_id = p.id
        WHERE
            pr.flash_promotion_id = ${flashPromotionId}
            AND pr.flash_promotion_session_id = #{sessionId}
    </select>
</mapper>`)
	vf.AddFile("src/main/java/com/macro/mall/portal/dao/HomeDao.java", `
package com.macro.mall.portal.dao;

import com.macro.mall.model.CmsSubject;
import com.macro.mall.model.PmsBrand;
import com.macro.mall.model.PmsProduct;
import com.macro.mall.portal.domain.FlashPromotionProduct;
import org.apache.ibatis.annotations.Param;

import java.util.List;

/**
 * 首页内容管理自定义Dao
 * Created by macro on 2019/1/28.
 */
public interface HomeDao {

    /**
     * 获取推荐品牌
     */
    List<PmsBrand> getRecommendBrandList(@Param("offset") Integer offset,@Param("limit") Integer limit);

    /**
     * 获取秒杀商品
     */
    List<FlashPromotionProduct> getFlashProductList(@Param("flashPromotionId") Long flashPromotionId, @Param("sessionId") Long sessionId);

    /**
     * 获取新品推荐
     */
    List<PmsProduct> getNewProductList(@Param("offset") Integer offset,@Param("limit") Integer limit);
    /**
     * 获取人气推荐
     */
    List<PmsProduct> getHotProductList(@Param("offset") Integer offset,@Param("limit") Integer limit);

    /**
     * 获取推荐专题
     */
    List<CmsSubject> getRecommendSubjectList(@Param("offset") Integer offset, @Param("limit") Integer limit);
}
`)

	t.Run("test get function name from xml", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf,
			`${*Dao.xml}.xpath("//mapper/*[contains(.,'${') and @id]/@id") as $url`,
			map[string][]string{
				"url": {`"getFlashProductList"`},
			}, false,
			// ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("test get function by name", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf,
			`getFlashProductList as $func`,
			map[string][]string{
				"func": {"Function-HomeDao.getFlashProductList"},
			}, false,
			// ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("test get function from xml", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf,
			`${*Dao.xml}.xpath("//mapper/*[contains(.,'${') and @id]/@id") as $url
			$url<searchFunc> as $func
			`,
			map[string][]string{
				"func": {"Function-HomeDao.getFlashProductList"},
			}, false,
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})
}

func TestMultipleResultExtraFile(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/resources/a.properties", `
	a.url=http://a.com
	`)
	vf.AddFile("src/resources/b.properties", `
	b.url=http://b.com
	`)
	ssatest.CheckSyntaxFlowWithFS(t, vf,
		`${*.properties}.re(/url=(.*)/) as $url`,
		map[string][]string{
			"url": {`"http://a.com"`, `"http://b.com"`},
		}, true,
		ssaapi.WithLanguage(ssaconfig.JAVA),
	)
}
