// demo1
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

// demo2
class Login {
  String hashPassword(char[] p) {
    return callHash(p);
  }

  public void doPrivilegedAction(String username, char[] password)
                                 throws SQLException {
    Connection connection = getConnection();
    if (connection == null) {
      // Handle error
    }
    try {
      String pwd = hashPassword(password);

      String sqlString = "SELECT * FROM db_user WHERE username = '"
                         + username +
                         "' AND password = '" + "" + "'";
      Statement stmt = connection.createStatement();
      ResultSet rs = stmt.executeQuery(sqlString);
    } finally {
    }
  }
}

// demo3
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;

class Login {
  String hashPassword(char[] p) {
    return callHash(p);
  }

  public void doPrivilegedAction(String username, char[] password)
                                 throws SQLException {
    Connection connection = getConnection();
    if (connection == null) {
      // Handle error
    }
    try {
      String pwd = hashPassword(password);

      String sqlString = "SELECT * FROM db_user WHERE username = '"
                         + username +
                         "' AND password = '" + pwd + "'";
      Statement stmt = connection.createStatement();
      ResultSet rs = stmt.executeQuery(sqlString);
    } finally {
    }
  }
}

// demo4
class Login {
  String hashPassword(char[] p) {
    return callHash(p);
  }

  public void doPrivilegedAction(String concatUser)
                                 throws SQLException {
    Connection connection = getConnection();
    if (connection == null) {
      // Handle error
    }
    try {
      String pwd = hashPassword(password);

      String sqlString = "SELECT * FROM db_user WHERE username = '";
      Statement stmt = connection.createStatement();
      ResultSet rs = stmt.executeQuery(sqlString.concat(concatUser));
    } finally {
    }
  }
}

// demo5
class Login {
  String hashPassword(char[] p) {
    return callHash(p);
  }

  public void doPrivilegedAction(String appendUser, String appendPass)
                                 throws SQLException {
    Connection connection = getConnection();
    if (connection == null) {
      // Handle error
    }
    try {
      String pwd = hashPassword(appendPass);

      StringBuilder sqlString = new StringBuilder();
      sqlString.append("SELECT * FROM db_user WHERE username = '");
      sqlString.append(appendUser);
      sqlString.append(" AND password = ");
      sqlString.append(pwd);
      Statement stmt = connection.createStatement();
      ResultSet rs = stmt.executeQuery(sqlString.toString());
    } finally {
    }
  }
}

// demo6
package com.ruoyi.common.datascope.aspect;

import java.util.ArrayList;
import java.util.List;
import org.aspectj.lang.JoinPoint;
import org.aspectj.lang.annotation.Aspect;
import org.aspectj.lang.annotation.Before;
import org.springframework.stereotype.Component;
import com.ruoyi.common.core.context.SecurityContextHolder;
import com.ruoyi.common.core.text.Convert;
import com.ruoyi.common.core.utils.StringUtils;
import com.ruoyi.common.core.web.domain.BaseEntity;
import com.ruoyi.common.datascope.annotation.DataScope;
import com.ruoyi.common.security.utils.SecurityUtils;
import com.ruoyi.system.api.domain.SysRole;
import com.ruoyi.system.api.domain.SysUser;
import com.ruoyi.system.api.model.LoginUser;

@Aspect
@Component
public class DataScopeAspect
{
    /**
     * 全部数据权限
     */
    public static final String DATA_SCOPE_ALL = "1";

    /**
     * 自定数据权限
     */
    public static final String DATA_SCOPE_CUSTOM = "2";

    /**
     * 部门数据权限
     */
    public static final String DATA_SCOPE_DEPT = "3";

    /**
     * 部门及以下数据权限
     */
    public static final String DATA_SCOPE_DEPT_AND_CHILD = "4";

    /**
     * 仅本人数据权限
     */
    public static final String DATA_SCOPE_SELF = "5";

    /**
     * 数据权限过滤关键字
     */
    public static final String DATA_SCOPE = "dataScope";

    @Before("@annotation(controllerDataScope)")
    public void doBefore(JoinPoint point, DataScope controllerDataScope) throws Throwable
    {
        clearDataScope(point);
        handleDataScope(point, controllerDataScope);
    }

    protected void handleDataScope(final JoinPoint joinPoint, DataScope controllerDataScope)
    {
        // 获取当前的用户
        LoginUser loginUser = SecurityUtils.getLoginUser();
        if (StringUtils.isNotNull(loginUser))
        {
            SysUser currentUser = loginUser.getSysUser();
            // 如果是超级管理员，则不过滤数据
            if (StringUtils.isNotNull(currentUser) && !currentUser.isAdmin())
            {
                String permission = StringUtils.defaultIfEmpty(controllerDataScope.permission(), SecurityContextHolder.getPermission());
                dataScopeFilter(joinPoint, currentUser, controllerDataScope.deptAlias(),
                        controllerDataScope.userAlias(), permission);
            }
        }
    }

    /**
     * 数据范围过滤
     *
     * @param joinPoint 切点
     * @param user 用户
     * @param deptAlias 部门别名
     * @param userAlias 用户别名
     * @param permission 权限字符
     */
    public static void dataScopeFilter(JoinPoint joinPoint, SysUser user, String deptAlias, String userAlias, String permission)
    {
        StringBuilder sqlString = new StringBuilder();
        List<String> conditions = new ArrayList<String>();
        List<String> scopeCustomIds = new ArrayList<String>();
        user.getRoles().forEach(role -> {
            if (DATA_SCOPE_CUSTOM.equals(role.getDataScope()) && StringUtils.containsAny(role.getPermissions(), Convert.toStrArray(permission)))
            {
                scopeCustomIds.add(Convert.toStr(role.getRoleId()));
            }
        });

        for (SysRole role : user.getRoles())
        {
            String dataScope = role.getDataScope();
            if (conditions.contains(dataScope))
            {
                continue;
            }
            if (!StringUtils.containsAny(role.getPermissions(), Convert.toStrArray(permission)))
            {
                continue;
            }
            if (DATA_SCOPE_ALL.equals(dataScope))
            {
                sqlString = new StringBuilder();
                conditions.add(dataScope);
                break;
            }
            else if (DATA_SCOPE_CUSTOM.equals(dataScope))
            {
                if (scopeCustomIds.size() > 1)
                {
                    // 多个自定数据权限使用in查询，避免多次拼接。
                    sqlString.append(StringUtils.format(" OR {}.dept_id IN ( SELECT dept_id FROM sys_role_dept WHERE role_id in ({}) ) ", deptAlias, String.join(",", scopeCustomIds)));
                }
                else
                {
                    sqlString.append(StringUtils.format(" OR {}.dept_id IN ( SELECT dept_id FROM sys_role_dept WHERE role_id = {} ) ", deptAlias, role.getRoleId()));
                }
            }
            else if (DATA_SCOPE_DEPT.equals(dataScope))
            {
                sqlString.append(StringUtils.format(" OR {}.dept_id = {} ", deptAlias, user.getDeptId()));
            }
            else if (DATA_SCOPE_DEPT_AND_CHILD.equals(dataScope))
            {
                sqlString.append(StringUtils.format(" OR {}.dept_id IN ( SELECT dept_id FROM sys_dept WHERE dept_id = {} or find_in_set( {} , ancestors ) )", deptAlias, user.getDeptId(), user.getDeptId()));
            }
            else if (DATA_SCOPE_SELF.equals(dataScope))
            {
                if (StringUtils.isNotBlank(userAlias))
                {
                    sqlString.append(StringUtils.format(" OR {}.user_id = {} ", userAlias, user.getUserId()));
                }
                else
                {
                    // 数据权限为仅本人且没有userAlias别名不查询任何数据
                    sqlString.append(StringUtils.format(" OR {}.dept_id = 0 ", deptAlias));
                }
            }
            conditions.add(dataScope);
        }

        // 角色都不包含传递过来的权限字符，这个时候sqlString也会为空，所以要限制一下,不查询任何数据
        if (StringUtils.isEmpty(conditions))
        {
            sqlString.append(StringUtils.format(" OR {}.dept_id = 0 ", deptAlias));
        }

        if (StringUtils.isNotBlank(sqlString.toString()))
        {
            Object params = joinPoint.getArgs()[0];
            if (StringUtils.isNotNull(params) && params instanceof BaseEntity)
            {
                BaseEntity baseEntity = (BaseEntity) params;
                baseEntity.getParams().put(DATA_SCOPE, " AND (" + sqlString.substring(4) + ")");
            }
        }
    }

    /**
     * 拼接权限sql前先清空params.dataScope参数防止注入
     */
    private void clearDataScope(final JoinPoint joinPoint)
    {
        Object params = joinPoint.getArgs()[0];
        if (StringUtils.isNotNull(params) && params instanceof BaseEntity)
        {
            BaseEntity baseEntity = (BaseEntity) params;
            baseEntity.getParams().put(DATA_SCOPE, "");
        }
    }
}

// demo7
import org.hibernate.Session;
import org.hibernate.SessionFactory;
import org.hibernate.cfg.Configuration;
import org.hibernate.query.Query;
import java.util.List;

public class UserManager {
    private SessionFactory sessionFactory;

    public UserManager() {
        // 初始化Hibernate SessionFactory
        try {
            sessionFactory = new Configuration().configure().buildSessionFactory();
        } catch (Throwable ex) {
            System.err.println("Failed to create sessionFactory object." + ex);
            throw new ExceptionInInitializerError(ex);
        }
    }

    // 用户实体类
    public static class User {
        private int id;
        private String username;
        private String email;
        private String role;

        // 构造函数、getter和setter方法省略
    }

    // 不安全的用户搜索方法 - 存在SQL注入风险
    public List<User> searchUsers(String searchTerm) {
        Session session = sessionFactory.openSession();
        try {
            // 危险：直接拼接用户输入到HQL查询中
            String hql = "FROM User WHERE username LIKE '%" + searchTerm + "%' OR email LIKE '%" + searchTerm + "%'";
            Query<User> query = session.createQuery(hql, User.class);
            return query.list();
        } finally {
            session.close();
        }
    }

    // 不安全的用户更新方法 - 存在SQL注入风险
    public void updateUserRole(int userId, String newRole) {
        Session session = sessionFactory.openSession();
        try {
            session.beginTransaction();
            // 危险：直接拼接用户输入到SQL查询中
            String sql = "UPDATE User SET role = '" + newRole + "' WHERE id = " + userId;
            session.createNativeQuery(sql).executeUpdate();
            session.getTransaction().commit();
        } catch (Exception e) {
            if (session.getTransaction() != null) {
                session.getTransaction().rollback();
            }
            e.printStackTrace();
        } finally {
            session.close();
        }
    }

    // 不安全的动态排序方法 - 存在SQL注入风险
    public List<User> getAllUsersSorted(String sortField, String sortOrder) {
        Session session = sessionFactory.openSession();
        try {
            // 危险：直接拼接用户输入到HQL查询中
            String hql = "FROM User ORDER BY " + sortField + " " + sortOrder;
            Query<User> query = session.createQuery(hql, User.class);
            return query.list();
        } finally {
            session.close();
        }
    }

    // 看似安全但仍有潜在风险的方法
    public User getUserByUsername(String username) {
        Session session = sessionFactory.openSession();
        try {
            String hql = "FROM User WHERE username = :username";
            Query<User> query = session.createQuery(hql, User.class);
            query.setParameter("username", username);
            return query.uniqueResult();
        } finally {
            session.close();
        }
    }
}

