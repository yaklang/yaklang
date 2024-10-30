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
            Result result = new StreamResult(resp.getWriter());
            transformerHandler.setResult(result);
        }catch (Exception e){
            e.printStackTrace();
        }

    }
}

