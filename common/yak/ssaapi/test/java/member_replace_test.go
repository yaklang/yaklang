package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestMemberReplace(t *testing.T) {
	code := `
package com.example.service;

import java.util.List;
import java.util.ArrayList;

public class JobService {
    private final Scheduler scheduler = new Scheduler();
    private final DynamicQuery dynamicQuery = new DynamicQuery();

    public void list(QuartzEntity quartz, int pageNo, int pageSize) throws SchedulerException {
        String countSql = "SELECT COUNT(*) FROM qrtz_cron_triggers";
        PageBean<QuartzEntity> data = new PageBean<>();
        StringBuffer nativeSql = new StringBuffer();
        Object[] params = new Object[]{};
        Pageable pageable = PageRequest.of(pageNo - 1, pageSize);
        List<QuartzEntity> list = dynamicQuery.nativeQueryPagingList(QuartzEntity.class, pageable, nativeSql.toString(), params);
        for (QuartzEntity quartzEntity : list) {
            JobKey key = new JobKey(quartzEntity.getJobName(), quartzEntity.getJobGroup());
            JobDetail jobDetail = scheduler.getJobDetail(key);
            quartzEntity.setJobMethodName(jobDetail.getJobDataMap().getString("jobMethodName"));
        }
        data = new PageBean<>(list, 0L);
    }
}

class Scheduler {
    JobDetail getJobDetail(JobKey key) { return new JobDetail(); }
}

class JobKey {
    JobKey(String name, String group) {}
}

class JobDetail {
    JobDataMap getJobDataMap() { return new JobDataMap(); }
}

class JobDataMap {
    String getString(String key) { return ""; }
}

class QuartzEntity {
    String getJobName() { return ""; }
    String getJobGroup() { return ""; }
    void setJobMethodName(String name) {}
}

class DynamicQuery {
    <T> List<T> nativeQueryPagingList(Class<T> type, Pageable pageable, String sql, Object[] params) { return new ArrayList<>(); }
}

class PageBean<T> {
    PageBean() {}
    PageBean(List<T> list, long totalCount) {}
}

class Pageable {}

class PageRequest {
    static Pageable of(int page, int size) { return new Pageable(); }
}

class SchedulerException extends Exception {}
`

	require.NotPanics(t, func() {
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.JAVA))
		require.NoError(t, err)
		require.NotNil(t, prog)
	})
}
