package com.range.demo.controller;

import com.range.demo.entity.PathInfo;
import com.range.demo.utils.ConvertsUtils;
import com.range.demo.utils.ResponseResult;
import com.range.demo.utils.ShellExcute;
import io.swagger.annotations.ApiOperation;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.*;

import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

@RestController
@RequestMapping("/command")
public class Commandi {
    private Logger logger = LoggerFactory.getLogger(getClass());

    @ApiOperation(value = "命令执行", notes = "processbuilder接受List参数")
    @PostMapping(value = "/processbuilder", produces = MediaType.APPLICATION_JSON_VALUE)
    public ResponseResult<String> start(@RequestBody PathInfo path) throws IOException {
        List<String> commands=new ArrayList<String>();
        commands.add("/bin/sh");
        commands.add("-c");
        commands.add(path.getPath());
        String result=ShellExcute.Start(commands);
        if (result != null) {
            return new ResponseResult<>(result, "执行成功", 200);
        }
        return new ResponseResult<>("result is null", "执行成功", 200);
    }


    @ApiOperation(value = "命令执行", notes = "exec接受string参数")
    @PostMapping(value = "/exec/string", produces = MediaType.APPLICATION_JSON_VALUE)
    public ResponseResult<String> execString(@RequestBody PathInfo path) throws IOException {
        String cmdStr;
        //1.日志注入 2.path本身校验防跨目录等等
        logger.info("Runtime.getRuntime().exec args:" + path);
        String dir=path.getPath();
        if(path.getType()==1)
        {
            cmdStr = dir;
        }else {
            cmdStr = "ping " + dir;
        }

        String result=ShellExcute.Exec(cmdStr);
        // p.getInputStream();
        if (result != null) {
            return new ResponseResult<>(result, "执行成功", 200);
        }
        //System.out.println(result);
        return new ResponseResult<>("result is null", "执行成功", 200);

    }

    @ApiOperation(value = "命令执行", notes = "exec接受array参数")
    @PostMapping(value = "/exec/array", produces = MediaType.APPLICATION_JSON_VALUE)
    public ResponseResult<String> execArray(@RequestBody PathInfo path) throws IOException {
        //安全检查path
        String dir=path.getPath();
        //base64 编码 echo touch /tmp/buglab|base64 || echo dG91Y2ggL3RtcC9idWdsYWIK|base64 -d
        String[] cmdStr = new String[]{"/bin/sh","-c",dir};
        logger.info("Runtime.getRuntime().exec args:" + ConvertsUtils.ArrayToString(cmdStr));
        String result=ShellExcute.Exec(cmdStr);
        // p.getInputStream();
        if (result != null) {
            return new ResponseResult<>(result, "执行成功", 200);
        }
        //System.out.println(result);
        return new ResponseResult<>("result is null", "执行成功", 200);

    }
}