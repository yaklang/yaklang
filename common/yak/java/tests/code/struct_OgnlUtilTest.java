/*
 * Copyright 2002-2006,2009 The Apache Software Foundation.
 * 
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 * 
 *      http://www.apache.org/licenses/LICENSE-2.0
 * 
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package com.opensymphony.xwork2.ognl;

import com.opensymphony.xwork2.ActionContext;
import com.opensymphony.xwork2.XWorkException;
import com.opensymphony.xwork2.XWorkTestCase;
import com.opensymphony.xwork2.conversion.impl.XWorkConverter;
import com.opensymphony.xwork2.interceptor.ChainingInterceptor;
import com.opensymphony.xwork2.test.User;
import com.opensymphony.xwork2.util.*;
import com.opensymphony.xwork2.util.reflection.ReflectionContextState;
import ognl.*;

import java.lang.reflect.Method;
import java.util.*;


/**
 * Unit test of {@link ognlUtil}.
 * 
 * @version $Date$ $Id$
 */
public class OgnlUtilTest extends XWorkTestCase {
    
    private OgnlUtil ognlUtil;
    
    @Override
    public void setUp() throws Exception {
        super.setUp();
        ognlUtil = container.getInstance(OgnlUtil.class);
    }
    


    public void testSetPropertiesBoolean() {
        Foo foo = new Foo();

        Map context = ognlUtil.createDefaultContext(foo);

        Map props = new HashMap();
        props.put("useful", "true");
        ognlUtil.setProperties(props, foo, context);

        assertEquals(true, foo.isUseful());

        props = new HashMap();
        props.put("useful", "false");
        ognlUtil.setProperties(props, foo, context);

        assertEquals(false, foo.isUseful());
    }

    public void testSetPropertiesDate() {
        Foo foo = new Foo();

        Map context = ognlUtil.createDefaultContext(foo);

        Map props = new HashMap();
        props.put("birthday", "02/12/1982");
        // US style test
        ognlUtil.setProperties(props, foo, context);

        Calendar cal = Calendar.getInstance();
        cal.clear();
        cal.set(Calendar.MONTH, Calendar.FEBRUARY);
        cal.set(Calendar.DAY_OF_MONTH, 12);
        cal.set(Calendar.YEAR, 1982);

        assertEquals(cal.getTime(), foo.getBirthday());
        
        //UK style test
        props.put("event", "18/10/2006 14:23:45");
        props.put("meeting", "09/09/2006 14:30");
        context.put(ActionContext.LOCALE, Locale.UK);

        ognlUtil.setProperties(props, foo, context);
        
        cal = Calendar.getInstance();
        cal.clear();
        cal.set(Calendar.MONTH, Calendar.OCTOBER);
        cal.set(Calendar.DAY_OF_MONTH, 18);
        cal.set(Calendar.YEAR, 2006);
        cal.set(Calendar.HOUR_OF_DAY, 14);
        cal.set(Calendar.MINUTE, 23);
        cal.set(Calendar.SECOND, 45);
        
        assertEquals(cal.getTime(), foo.getEvent());
        
        cal = Calendar.getInstance();
        cal.clear();
        cal.set(Calendar.MONTH, Calendar.SEPTEMBER);
        cal.set(Calendar.DAY_OF_MONTH, 9);
        cal.set(Calendar.YEAR, 2006);
        cal.set(Calendar.HOUR_OF_DAY, 14);
        cal.set(Calendar.MINUTE, 30);
        
        assertEquals(cal.getTime(), foo.getMeeting());
        
        //test RFC 3339 date format for JSON
        props.put("event", "1996-12-19T16:39:57Z");
        ognlUtil.setProperties(props, foo, context);
        
        cal = Calendar.getInstance();
        cal.clear();
        cal.set(Calendar.MONTH, Calendar.DECEMBER);
        cal.set(Calendar.DAY_OF_MONTH, 19);
        cal.set(Calendar.YEAR, 1996);
        cal.set(Calendar.HOUR_OF_DAY, 16);
        cal.set(Calendar.MINUTE, 39);
        cal.set(Calendar.SECOND, 57);
        
        assertEquals(cal.getTime(), foo.getEvent());
        
        //test setting a calendar property
        props.put("calendar", "1996-12-19T16:39:57Z");
        ognlUtil.setProperties(props, foo, context);
        assertEquals(cal, foo.getCalendar());
    }

    public void testSetPropertiesInt() {
        Foo foo = new Foo();

        Map context = ognlUtil.createDefaultContext(foo);

        Map props = new HashMap();
        props.put("number", "2");
        ognlUtil.setProperties(props, foo, context);

        assertEquals(2, foo.getNumber());
    }

    public void testSetPropertiesLongArray() {
        Foo foo = new Foo();

        Map context = ognlUtil.createDefaultContext(foo);

        Map props = new HashMap();
        props.put("points", new String[]{"1", "2"});
        ognlUtil.setProperties(props, foo, context);

        assertNotNull(foo.getPoints());
        assertEquals(2, foo.getPoints().length);
        assertEquals(1, foo.getPoints()[0]);
        assertEquals(2, foo.getPoints()[1]);
    }

    public void testSetPropertiesString() {
        Foo foo = new Foo();

        Map context = ognlUtil.createDefaultContext(foo);

        Map props = new HashMap();
        props.put("title", "this is a title");
        ognlUtil.setProperties(props, foo, context);

        assertEquals(foo.getTitle(), "this is a title");
    }

    public void testSetProperty() {
        Foo foo = new Foo();
        Map context = ognlUtil.createDefaultContext(foo);
        assertFalse(123456 == foo.getNumber());
        ognlUtil.setProperty("number", "123456", foo, context);
        assertEquals(123456, foo.getNumber());
    }


    public void testSetList() throws Exception {
        ChainingInterceptor foo = new ChainingInterceptor();
        ChainingInterceptor foo2 = new ChainingInterceptor();

        OgnlContext context = (OgnlContext) ognlUtil.createDefaultContext(null);
        SimpleNode expression = (SimpleNode) Ognl.parseExpression("{'a','ruby','b','tom'}");


        Ognl.getValue(expression, context, "aksdj");

        final ValueStack stack = ActionContext.getContext().getValueStack();

        Object result = Ognl.getValue(ognlUtil.compile("{\"foo\",'ruby','b','tom'}"), context, foo);
        foo.setIncludes((Collection) result);

        assertEquals(4, foo.getIncludes().size());
        assertEquals("foo", foo.getIncludes().toArray()[0]);
        assertEquals("ruby", foo.getIncludes().toArray()[1]);
        assertEquals("b", "" + foo.getIncludes().toArray()[2]);
        assertEquals("tom", foo.getIncludes().toArray()[3]);

        Object result2 = Ognl.getValue(ognlUtil.compile("{\"foo\",'ruby','b','tom'}"), context, foo2);
        ognlUtil.setProperty("includes", result2, foo2, context);

        assertEquals(4, foo.getIncludes().size());
        assertEquals("foo", foo.getIncludes().toArray()[0]);
        assertEquals("ruby", foo.getIncludes().toArray()[1]);
        assertEquals("b", "" + foo.getIncludes().toArray()[2]);
        assertEquals("tom", foo.getIncludes().toArray()[3]);

        result = ActionContext.getContext().getValueStack().findValue("{\"foo\",'ruby','b','tom'}");

        foo.setIncludes((Collection) result);
        assertEquals(ArrayList.class, result.getClass());

        assertEquals(4, foo.getIncludes().size());
        assertEquals("foo", foo.getIncludes().toArray()[0]);
        assertEquals("ruby", foo.getIncludes().toArray()[1]);
        assertEquals("b", "" + foo.getIncludes().toArray()[2]);
        assertEquals("tom", foo.getIncludes().toArray()[3]);
    }


    public void testStringToLong() {
        Foo foo = new Foo();

        Map context = ognlUtil.createDefaultContext(foo);

        Map props = new HashMap();
        props.put("aLong", "123");

        ognlUtil.setProperties(props, foo, context);
        assertEquals(123, foo.getALong());

        props.put("aLong", new String[]{"123"});

        foo.setALong(0);
        ognlUtil.setProperties(props, foo, context);
        assertEquals(123, foo.getALong());
    }

    public void testNullProperties() {
        Foo foo = new Foo();
        foo.setALong(88);

        Map context = ognlUtil.createDefaultContext(foo);

        ognlUtil.setProperties(null, foo, context);
        assertEquals(88, foo.getALong());

        Map props = new HashMap();
        props.put("aLong", "99");
        ognlUtil.setProperties(props, foo, context);
        assertEquals(99, foo.getALong());
    }
    
    public void testCopyNull() {
        Foo foo = new Foo();
        Map context = ognlUtil.createDefaultContext(foo);
   		ognlUtil.copy(null, null, context);

   		ognlUtil.copy(foo, null, context);
   		ognlUtil.copy(null, foo, context);
    }
    
    public void testGetTopTarget() throws Exception {
        Foo foo = new Foo();
        Map context = ognlUtil.createDefaultContext(foo);

        CompoundRoot root = new CompoundRoot();
        Object top = ognlUtil.getRealTarget("top", context, root);
        assertEquals(root, top); // top should be root
        
        root.push(foo);
        Object val = ognlUtil.getRealTarget("unknown", context, root);
        assertNull(val); // not found
    }
    
    public void testGetBeanMap() throws Exception {
    	Bar bar = new Bar();
    	bar.setTitle("I have beer");
        
    	Foo foo = new Foo();
        foo.setALong(123);
        foo.setNumber(44);
        foo.setBar(bar);
        foo.setTitle("Hello Santa");
        foo.setUseful(true);
        
        // just do some of the 15 tests
        Map beans = ognlUtil.getBeanMap(foo);
        assertNotNull(beans);
        assertEquals(19, beans.size());
        assertEquals("Hello Santa", beans.get("title"));
        assertEquals(new Long("123"), beans.get("ALong"));
        assertEquals(new Integer("44"), beans.get("number"));
        assertEquals(bar, beans.get("bar"));
        assertEquals(Boolean.TRUE, beans.get("useful"));
    }

    public void testGetBeanMapNoReadMethod() throws Exception {
    	MyWriteBar bar = new MyWriteBar();
    	bar.setBar("Sams");
    	
    	Map beans = ognlUtil.getBeanMap(bar);
    	assertEquals(2, beans.size());
    	assertEquals(new Integer("1"), beans.get("id"));
    	assertEquals("There is no read method for bar", beans.get("bar"));
    }

    /**
	 * XW-281
	 */
    public void testSetBigIndexedValue() {
        ValueStack stack = ActionContext.getContext().getValueStack();
        Map stackContext = stack.getContext();
        stackContext.put(ReflectionContextState.CREATE_NULL_OBJECTS, Boolean.FALSE);
        stackContext.put(ReflectionContextState.DENY_METHOD_EXECUTION, Boolean.TRUE);
        stackContext.put(XWorkConverter.REPORT_CONVERSION_ERRORS, Boolean.TRUE);

        User user = new User();
        stack.push(user);

        // indexed string w/ existing array
        user.setList(new ArrayList());

        String[] foo = new String[]{"asdf"};
        ((OgnlValueStack)stack).setDevMode("true");
        try {
            stack.setValue("list.1114778947765", foo);
            fail("non-valid expression: list.1114778947765"); 
        }
        catch(RuntimeException ex) {
            ; // it's oke
        }
        
        try {
            stack.setValue("1114778947765", foo);
            fail("non-valid expression: 1114778947765"); 
        }
        catch(RuntimeException ex) {
            ;
        }
        
        try {
            stack.setValue("1234", foo);
            fail("non-valid expression: 1114778947765"); 
        }
        catch(RuntimeException ex) {
            ;
        }
        
        ((OgnlValueStack)stack).setDevMode("false");
        stack.setValue("list.1114778947765", foo);
        stack.setValue("1114778947765", foo);
        stack.setValue("1234", foo);
    }

    public void testAvoidCallingMethodsOnObjectClass() throws Exception {
        Foo foo = new Foo();

        Exception expected = null;
        try {
            ognlUtil.setExcludedClasses(Object.class.getName());
            ognlUtil.setValue("class.classLoader.defaultAssertionStatus", ognlUtil.createDefaultContext(foo), foo, true);
            fail();
        } catch (OgnlException e) {
            expected = e;
        }
        assertNotNull(expected);
        assertSame(NoSuchPropertyException.class, expected.getClass());
        assertEquals("com.opensymphony.xwork2.util.Foo.class", expected.getMessage());
    }


    public void testAvoidCallingMethodsOnObjectClassAsMap() throws Exception {
        Foo foo = new Foo();

        Exception expected = null;
        try {
            ognlUtil.setExcludedClasses(Object.class.getName());
            ognlUtil.setValue("class['classLoader']['defaultAssertionStatus']", ognlUtil.createDefaultContext(foo), foo, true);
            fail();
        } catch (OgnlException e) {
            expected = e;
        }
        assertNotNull(expected);
        assertSame(NoSuchPropertyException.class, expected.getClass());
        assertEquals("com.opensymphony.xwork2.util.Foo.class", expected.getMessage());
    }

    public void testAvoidCallingMethodsOnObjectClassAsMap2() throws Exception {
        Foo foo = new Foo();

        Exception expected = null;
        try {
            ognlUtil.setValue("foo['class']['classLoader']['defaultAssertionStatus']", ognlUtil.createDefaultContext(foo), foo, true);
            fail();
        } catch (OgnlException e) {
            expected = e;
        }
        assertNotNull(expected);
        assertSame(NoSuchPropertyException.class, expected.getClass());
        assertEquals("com.opensymphony.xwork2.util.Foo.foo", expected.getMessage());
    }

    public void testAvoidCallingMethodsWithBraces() throws Exception {
        Foo foo = new Foo();

        Exception expected = null;
        try {
            ognlUtil.setValue("toString()", ognlUtil.createDefaultContext(foo), foo, true);
            fail();
        } catch (OgnlException e) {
            expected = e;
        }
        assertNotNull(expected);
        assertSame(InappropriateExpressionException.class, expected.getClass());
        assertEquals(expected.getMessage(), "Inappropriate OGNL expression: toString()");
    }

    public void testAvoidCallingSomeClasses() throws Exception {
        Foo foo = new Foo();

        Exception expected = null;
        try {
            ognlUtil.setExcludedClasses(Runtime.class.getName());
            ognlUtil.setValue("@java.lang.Runtime@getRuntime().exec('mate')", ognlUtil.createDefaultContext(foo), foo, true);
            fail();
        } catch (OgnlException e) {
            expected = e;
        }
        assertNotNull(expected);
        assertSame(MethodFailedException.class, expected.getClass());
        assertEquals(expected.getMessage(), "Method \"getRuntime\" failed for object class java.lang.Runtime");
    }

    public static class Email {
        String address;

        public void setAddress(String address) {
            this.address = address;
        }

        @Override
        public String toString() {
            return address;
        }
    }

    static class TestObject {
        private Integer myIntegerProperty;
        private Long myLongProperty;
        private String myStrProperty;

        public void setMyIntegerProperty(Integer myIntegerProperty) {
            this.myIntegerProperty = myIntegerProperty;
        }

        public String getMyIntegerProperty() {
            return myIntegerProperty.toString();
        }

        public void setMyLongProperty(Long myLongProperty) {
            this.myLongProperty = myLongProperty;
        }

        public Long getMyLongProperty() {
            return myLongProperty;
        }

        public void setMyStrProperty(String myStrProperty) {
            this.myStrProperty = myStrProperty;
        }

        public String getMyStrProperty() {
            return myStrProperty;
        }
    }

    class EmailAction {
        public List email = new OgnlList(Email.class);

        public List getEmail() {
            return this.email;
        }
    }

    class OgnlList extends ArrayList {
        private Class clazz;

        public OgnlList(Class clazz) {
            this.clazz = clazz;
        }

        @Override
        public synchronized Object get(int index) {
            while (index >= this.size()) {
                try {
                    this.add(clazz.newInstance());
                } catch (Exception e) {
                    throw new XWorkException(e);
                }
            }

            return super.get(index);
        }
    }
    
    private class MyWriteBar {
    	private int id;
    	
    	public int getId() {
    		return id;
    	}
    	
    	public void setBar(String name) {
    		if ("Sams".equals(name))
    			id = 1;
    		else
    			id = 999;
    	}
    	
    }
}
