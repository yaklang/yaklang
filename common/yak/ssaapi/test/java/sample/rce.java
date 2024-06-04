package com.vuln.controller;

import com.google.common.base.Joiner;
import com.google.common.base.Preconditions;
import org.apache.commons.lang3.StringUtils;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

@RestController
@RequestMapping(value = "/rce")
public class RceController {

    @RequestMapping(value = "one")
    public StringBuffer One(@RequestParam(value = "command") String command) {
        StringBuffer sb = new StringBuffer();
        List<String> commands = new ArrayList<>();
        commands.add(command);

        ProcessBuilder processBuilder = new ProcessBuilder(commands);
        processBuilder.redirectErrorStream(true);
        try {
            Process process = processBuilder.start();

            BufferedReader br = new BufferedReader(new InputStreamReader(process.getInputStream()));
            String line;
            while ((line = br.readLine()) != null) {
                sb.append(line);
            }
            br.close();
        } catch (Exception e) {

        }
        return sb;
    }

    @RequestMapping(value = "two")
    public StringBuffer Two(@RequestParam(value="command") String command) {
        String cmd = "";
        StringBuffer result = new StringBuffer();
        try {
            cmd = String.format("%s", command);
            System.out.println(cmd);
            Process process = Runtime.getRuntime().exec(cmd);
            InputStream stdIn = process.getInputStream();
            InputStreamReader isr = new InputStreamReader(stdIn);
            String line = null;
            BufferedReader br = new BufferedReader(isr);
            while ((line = br.readLine()) != null){
                result.append(line + "\n");
            }
            boolean success = process.waitFor(50, TimeUnit.SECONDS);
        } catch (Throwable e) {
            return null;
        }
        System.out.println(result);
        return result;
    }
}