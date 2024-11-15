package buildin_rule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var Cases = []BuildinRuleTestCase{
	{
		Name: "检测Java任意文件下载(attachment)",
		Rule: `java-filedownload-attachment-filename`,
		FS: map[string]string{
			"download.java": "download.java",
		},
		ContainsAll: []string{"attachment", "filename"},
	},
	{
		Name: "XStream 基础使用",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream.java": "xstream.java",
		},
		ContainsAll: []string{"xstream.fromXML"},
	},

	{
		Name: "XStream 类成员 - 基础使用",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream-2.java": "xstream-2.java",
		},
		ContainsAll: []string{"xstreamInstance.fromXML"},
	},

	{
		Name: "XStream 类成员(空成员) - 基础使用",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream-3.java": "xstream-3.java",
		},
		ContainsAll: []string{"xstreamInstance.fromXML"},
	},

	{
		Name: "XStream 基础使用(negative)",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream-safe.java": "xstream-safe.java",
		},
		NegativeTest: true,
	},

	{
		Name: "SAXBuilder 基础使用(安全)",
		Rule: `java-saxbuilder-unsafe`,
		FS: map[string]string{
			"saxbuilder-safe.java": "saxbuilder-safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXBuilder 基础使用不安全",
		Rule: `java-saxbuilder-unsafe`,
		FS: map[string]string{
			"saxbuilder-unsafe.java": "saxbuilder-unsafe.java",
		},
		NegativeTest: false,
		ContainsAll:  []string{"SAXBuilder"},
	},
	{
		Name: "SAXParserFactory 基础检查",
		Rule: `java-saxparser-factory-unsafe`,
		FS: map[string]string{
			"saxparser-factory-unsafe.java": "saxparser-factory-unsafe.java",
		},
		NegativeTest: false,
		ContainsAll:  []string{"SAXParserFactory"},
	},
	{
		Name: "SAXParserFactory 基础检查(安全)",
		Rule: `java-saxparser-factory-unsafe`,
		FS: map[string]string{
			"saxparser-factory-safe.java": "saxparser-factory-safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXReader 基础检查(安全)",
		Rule: `java-saxreader-unsafe`,
		FS: map[string]string{
			"saxreazder.java": "saxreader/safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXReader 基础检查(不安全)",
		Rule: `java-saxreader-unsafe`,
		FS: map[string]string{
			"saxreazder.java": "saxreader/unsafe.java",
		},
		ContainsAll: []string{"SAXReader"},
	},

	{
		Name: "XMLReaderFactory 基础检查(不安全)",
		Rule: `java-xmlreader-factory-unsafe`,
		FS: map[string]string{
			"xmlreaderfactory.java": "org-xml-sax-xmlreader/unsafe.java",
		},
		ContainsAll: []string{"createXMLReade", "example.xml"},
	},
	{
		Name: "XMLReaderFactory 基础检查(消极测试)",
		Rule: `java-xmlreader-factory-unsafe`,
		FS: map[string]string{
			"xmlreaderfactory.java": "org-xml-sax-xmlreader/safe.java",
		},
		NegativeTest: true,
	},
}

func TestVerifiedRule(t *testing.T) {
	yakit.InitialDatabase()
	for rule := range sfdb.YieldSyntaxFlowRules(consts.GetGormProfileDatabase(), context.Background()) {
		f, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
		if err != nil {
			t.Fatalf("compile rule %s error: %s", rule.RuleName, err)
		}
		if len(f.VerifyFs) > 0 || len(f.NegativeFs) > 0 {
			t.Run(strings.Join(append(strings.Split(rule.Tag, "|"), rule.RuleName), "/"), func(t *testing.T) {
				t.Log("Start to verify: " + rule.RuleName)
				err := ssatest.EvaluateVerifyFilesystemWithRule(rule, t)
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestBuildInRule(t *testing.T) {
	for i := 0; i < len(Cases); i++ {
		c := Cases[i]
		run(t, c.Name, c)
	}
}

func TestBuildInRule_DEBUG(t *testing.T) {
	if utils.InGithubActions() {
		t.SkipNow()
		return
	}

	var name = "php-custom_param.sf"

	for i := 0; i < len(Cases); i++ {
		c := Cases[i]

		if !utils.MatchAllOfSubString(c.Name, name) {
			t.Log("skip " + c.Name)
			continue
		}
		run(t, c.Name, c)
	}
}

func TestBuildInRule_Verify_DEBUG(t *testing.T) {
	if utils.InGithubActions() {
		t.SkipNow()
		return
	}

	yakit.InitialDatabase()
	ruleName := "java-path-travel-directly.sf"

	rule, err := sfdb.GetRulePure(ruleName)
	if err != nil {
		t.Fatal(err)
	}

	f, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.VerifyFs) > 0 || len(f.NegativeFs) > 0 {
		t.Run(rule.RuleName, func(t *testing.T) {
			t.Log("Start to verify: " + rule.RuleName)
			err := ssatest.EvaluateVerifyFilesystemWithRule(rule, t)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuildInRule_Verify_Negative_AlertMin(t *testing.T) {
	err := ssatest.EvaluateVerifyFilesystem(`
desc(
alert_min: '2',
language: yaklang,
'file://a.yak': <<<EOF
b = () => {
	a = 1;
}
EOF
)

a as $output;
check $output;
alert $output;

`, t)
	if err == nil {
		t.Fatal("expect error")
	}
}

func TestBuildInRule_Verify_Positive_AlertMin2(t *testing.T) {
	err := ssatest.EvaluateVerifyFilesystem(`
desc(
alert_min: 1,
language: yaklang,
'file://a.yak': <<<EOF
b = () => {
	a = 1;
}
EOF
)

a as $output;
check $output;
alert $output;

`, t)
	if err != nil {
		t.Fatal(err)
	}
}

func TestImport(t *testing.T) {
	_, err := sfdb.ImportRuleWithoutValid("test.sf", `
desc(
	title: "import test",
	level: "high",
	lang: "php",
)
$a #-> * as $param

alert $param for {"level": "high"}
`, true)
	require.NoError(t, err)
	rule, err := sfdb.GetRule("test.sf")
	require.NoError(t, err)
	var m map[string]*schema.SyntaxFlowDescInfo
	fmt.Println(rule.AlertDesc)
	err = json.Unmarshal(codec.AnyToBytes(rule.AlertDesc), &m)
	require.NoError(t, err)
	info, ok := m["param"]
	require.True(t, ok)
	require.True(t, info.Severity == schema.SFR_SEVERITY_HIGH)
	err = sfdb.DeleteRuleByRuleName("test.sf")
	require.NoError(t, err)
}

func TestJavaDependencies(t *testing.T) {
	code := `
__dependency__.*fastjson.version as $ver;
$ver?{version_in:(1.2.3,2.3.4]}  as $vulnVersion
alert $vulnVersion for {
	title:"存在fastjson 1.2.3-2.3.4漏洞",
};

desc(
lang: java,
'file://pom.xml': <<<CODE
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>vulnerable-fastjson-app</artifactId>
    <version>1.0-SNAPSHOT</version>

    <dependencies>
        <!-- Fastjson dependency with known vulnerabilities -->
        <dependency>
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <!-- An example version with known vulnerabilities, make sure to check for specific vulnerable versions -->
            <version>1.2.24</version>
        </dependency>
    </dependencies>
</project>
CODE
)`
	err := ssatest.EvaluateVerifyFilesystem(code, t)
	if err != nil {
		t.Fatal(err)
	}
}

const DEBUGCODE = `
desc(
    title: "Check for suspected SQL statement concatenation and execution in database queries",
    title_zh: "检查疑似 SQL 语句拼接并执行到数据库查询的代码"
)

e"SELECT COUNT(*) FROM qrtz_cron_triggers"<show> as $a;
alert $a

/(?i)\w+sql/ as $b;
alert $b


desc(
lang: java,
"file://a.java": <<<FILE
package com.itstyle.quartz.service.impl;


@Service("jobService")
public class JobServiceImpl implements IJobService {

	@Autowired
	private DynamicQuery dynamicQuery;
    @Autowired
    private Scheduler scheduler;
	@Override
	public Result listQuartzEntity(QuartzEntity quartz,
			Integer pageNo, Integer pageSize) throws SchedulerException {
	    String countSql = "SELECT COUNT(*) FROM qrtz_cron_triggers";
        if(!StringUtils.isEmpty(quartz.getJobName())){
            countSql+=" AND job.JOB_NAME = "+quartz.getJobName();
        }
        Long totalCount = dynamicQuery.nativeQueryCount(countSql);
        PageBean<QuartzEntity> data = new PageBean<>();
        if(totalCount>0){
            StringBuffer nativeSql = new StringBuffer();
            nativeSql.append("SELECT job.JOB_NAME as jobName,job.JOB_GROUP as jobGroup,job.DESCRIPTION as description,job.JOB_CLASS_NAME as jobClassName,");
            nativeSql.append("cron.CRON_EXPRESSION as cronExpression,tri.TRIGGER_NAME as triggerName,tri.TRIGGER_STATE as triggerState,");
            nativeSql.append("job.JOB_NAME as oldJobName,job.JOB_GROUP as oldJobGroup ");
            nativeSql.append("FROM qrtz_job_details AS job ");
            nativeSql.append("LEFT JOIN qrtz_triggers AS tri ON job.JOB_NAME = tri.JOB_NAME  AND job.JOB_GROUP = tri.JOB_GROUP ");
            nativeSql.append("LEFT JOIN qrtz_cron_triggers AS cron ON cron.TRIGGER_NAME = tri.TRIGGER_NAME AND cron.TRIGGER_GROUP= tri.JOB_GROUP ");
            nativeSql.append("WHERE tri.TRIGGER_TYPE = 'CRON'");
            Object[] params = new  Object[]{};
            if(!StringUtils.isEmpty(quartz.getJobName())){
                nativeSql.append(" AND job.JOB_NAME = ?");
                params = new Object[]{quartz.getJobName()};
            }
            Pageable pageable = PageRequest.of(pageNo-1,pageSize);
            List<QuartzEntity> list = dynamicQuery.nativeQueryPagingList(QuartzEntity.class,pageable, nativeSql.toString(), params);
            for (QuartzEntity quartzEntity : list) {
                JobKey key = new JobKey(quartzEntity.getJobName(), quartzEntity.getJobGroup());
                JobDetail jobDetail = scheduler.getJobDetail(key);
                quartzEntity.setJobMethodName(jobDetail.getJobDataMap().getString("jobMethodName"));
            }
            data = new PageBean<>(list, totalCount);
        }
        return Result.ok(data);
	}

	@Override
	public Long listQuartzEntity(QuartzEntity quartz) {
		StringBuffer nativeSql = new StringBuffer();
		nativeSql.append("SELECT COUNT(*)");
		nativeSql.append("FROM qrtz_job_details AS job LEFT JOIN qrtz_triggers AS tri ON job.JOB_NAME = tri.JOB_NAME ");
		nativeSql.append("LEFT JOIN qrtz_cron_triggers AS cron ON cron.TRIGGER_NAME = tri.TRIGGER_NAME ");
		nativeSql.append("WHERE tri.TRIGGER_TYPE = 'CRON'");
		return dynamicQuery.nativeQueryCount(nativeSql.toString(), new Object[]{});
	}

    @Override
    @Transactional
    public void save(QuartzEntity quartz) throws Exception{
        //如果是修改  展示旧的 任务
        if(quartz.getOldJobGroup()!=null){
            JobKey key = new JobKey(quartz.getOldJobName(),quartz.getOldJobGroup());
            scheduler.deleteJob(key);
        }
        Class cls = Class.forName(quartz.getJobClassName()) ;
        cls.newInstance();
        //构建job信息
        JobDetail job = JobBuilder.newJob(cls).withIdentity(quartz.getJobName(),
                quartz.getJobGroup())
                .withDescription(quartz.getDescription()).build();
        job.getJobDataMap().put("jobMethodName", quartz.getJobMethodName());
        // 触发时间点
        CronScheduleBuilder cronScheduleBuilder = CronScheduleBuilder.cronSchedule(quartz.getCronExpression());
        Trigger trigger = TriggerBuilder.newTrigger().withIdentity("trigger"+quartz.getJobName(), quartz.getJobGroup())
                .startNow().withSchedule(cronScheduleBuilder).build();
        //交由Scheduler安排触发
        scheduler.scheduleJob(job, trigger);
    }
}
FILE,
)
`

func TestJavaDEBUG(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	err := ssatest.EvaluateVerifyFilesystem(DEBUGCODE, t)
	if err != nil {
		t.Fatal(err)
	}
}
