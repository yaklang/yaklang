package com.itstyle.quartz.job;

import com.itstyle.quartz.entity.*;
import com.itstyle.quartz.service.IDetailsBeanService;
import com.itstyle.quartz.service.IMogudingService;
import com.itstyle.quartz.service.ISignInLogService;
import com.itstyle.quartz.service.IUserinfoService;
import com.itstyle.quartz.utils.ApplicationContextUtil;
import com.itstyle.quartz.utils.DateUtil;
import org.quartz.*;

import java.io.Serializable;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.util.List;

 /*
 * @DisallowConcurrentExecution 保证上一个任务执行完后，再去执行下一个任务，这里的任务是同一个任务
 */
@DisallowConcurrentExecution
public class ChickenJob implements Job, Serializable {

    private static final long serialVersionUID = 1L;

    @Override
    public void execute(JobExecutionContext context) {
        JobDetail jobDetail = context.getJobDetail();
        JobDataMap dataMap = jobDetail.getJobDataMap();
        /**
         * 获取任务中保存的方法名字，动态调用方法
         */
        String methodName = dataMap.getString("jobMethodName");
        try {
            ChickenJob job = new ChickenJob();
            Method method = job.getClass().getMethod(methodName);
            method.invoke(job);
        } catch (NoSuchMethodException e) {
            e.printStackTrace();
        } catch (IllegalAccessException e) {
            e.printStackTrace();
        } catch (InvocationTargetException e) {
            e.printStackTrace();
        }
    }
}