package com.example;

import org.dom4j.Document;
import org.dom4j.Element;
import org.dom4j.io.SAXReader;
import org.xml.sax.EntityResolver;
import org.xml.sax.InputSource;

import java.io.File;
import java.io.StringReader;

public class SafeSAXReaderExample {
    public static void main(String[] args) {
        SAXReader saxReader = new SAXReader();

        // 禁用外部实体
        try {
            saxReader.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
            saxReader.setFeature("http://xml.org/sax/features/external-general-entities", false);
            saxReader.setFeature("http://xml.org/sax/features/external-parameter-entities", false);
        } catch (Exception e) {
            e.printStackTrace();
            return;
        }

        // 使用 EntityResolver 禁用实体解析
        EntityResolver noop = (publicId, systemId) -> new InputSource(new StringReader(""));
        saxReader.setEntityResolver(noop);

        // 禁用 DTD
        saxReader.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);

        try {
            Document document = saxReader.read(new File("D:\\example.xml"));
            Element rootElement = document.getRootElement();
            System.out.println("Root Element: " + rootElement.getName());
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}