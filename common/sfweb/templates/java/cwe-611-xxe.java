// demo1
package com.example.sax;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.xml.XMLConstants;
import javax.xml.transform.Result;
import javax.xml.transform.sax.SAXTransformerFactory;
import javax.xml.transform.sax.TransformerHandler;
import javax.xml.transform.stream.StreamResult;
import javax.xml.transform.stream.StreamSource;
import java.io.IOException;

public class SAXTransformerFactoryServlet extends HttpServlet {
    private void postNoFixXxe(HttpServletRequest req, HttpServletResponse resp){
        try{
            SAXTransformerFactory sf = (SAXTransformerFactory) SAXTransformerFactory.newInstance();
            StreamSource source = new StreamSource(req.getReader());
            TransformerHandler transformerHandler = sf.newTransformerHandler(source);
            // 创建Result对象，并通过transformerHandler将目的流与其关联
            Result result = new StreamResult(resp.getWriter());
            transformerHandler.setResult(result);
        }catch (Exception e){
            e.printStackTrace();
        }

    }
}

// demo 2
import javax.xml.transform.Transformer;
import javax.xml.transform.TransformerException;
import javax.xml.transform.TransformerFactory;
import javax.xml.transform.stream.StreamResult;
import javax.xml.transform.stream.StreamSource;
import java.io.File;
import java.io.IOException;

public class XXEVulnerableExample {
    public static void main(String[] args) {
        try {
            TransformerFactory transformerFactory = TransformerFactory.newInstance();
            Transformer transformer = transformerFactory.newTransformer(
                    new StreamSource(new File("vulnerable.xsl")));
            transformer.transform(
                    new StreamSource(new File("input.xml")),
                    new StreamResult(new File("output.xml")));
        } catch (TransformerException | IOException e) {
            e.printStackTrace();
        }
    }
}

