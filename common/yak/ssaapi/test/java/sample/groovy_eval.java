package org.vuln.javasec.controller.basevul.rce;
import groovy.lang.GroovyShell;
import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;

@Controller
@RequestMapping("/home/rce")
public class GroovyExecIF {

    @GetMapping("/groovy")
    public String groovyExec(String cmd, Model model) {
        GroovyShell shell = new GroovyShell();
        try {
            shell.evaluate(cmd);
            model.addAttribute("results", "执行成功！！！");
        } catch (Exception e) {
            e.printStackTrace();
            model.addAttribute("results", e.toString());
        }
        return "basevul/rce/groovy";
    }
}