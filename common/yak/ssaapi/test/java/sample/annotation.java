package com.vuln.controller;

public class DemoABCEntryClass {
    @RequestMapping(value = "/one")
    public String methodEntry(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        return "Hello World" + xmlStr;
    }
}