// package org.vuln.javasec.controller.basevul.rce;
package org.vuln.javasec.controller.basevul.file;

import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.util.HtmlUtils;

import javax.script.Bindings;
import javax.script.ScriptContext;
import javax.script.ScriptEngine;
import javax.script.ScriptEngineManager;

@Controller
@RequestMapping("/home/rce")
public class LoadJsExec {
    @GetMapping("/loadjs")
    public String loadJsExec(String url, Model model) {
        try {
            ScriptEngine engine = new ScriptEngineManager().getEngineByExtension("js");

            // Bindings：用来存放数据的容器
            Bindings bindings = engine.getBindings(ScriptContext.ENGINE_SCOPE);
            String payload = String.format("load('%s')", url);
            engine.eval(payload, bindings);
            model.addAttribute("results", "远程脚本: " + HtmlUtils.htmlEscape(url) + " 执行成功!");
        } catch (Exception e) {
            e.printStackTrace();
            model.addAttribute("results", e.toString());
        }
        return "basevul/rce/loadjs";
    }
}