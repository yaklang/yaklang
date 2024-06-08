package com.vuln.controller;

public class DemoABCEntryClass {
    public String methodEntry(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        return "Hello World" + xmlStr;
    }
}