package com.example;

import org.dom4j.Document;
import org.dom4j.Element;
import org.dom4j.io.SAXReader;

import java.io.File;

public class UnsafeSAXReaderExample {
    public static void main(String[] args) {
        SAXReader saxReader = new SAXReader();
        try {
            Document document = saxReader.read(new File("D:\\example.xml"));
            Element rootElement = document.getRootElement();
            System.out.println("Root Element: " + rootElement.getName());
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}