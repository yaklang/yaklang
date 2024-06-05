//
// Source code recreated from a .class file by IntelliJ IDEA
// (powered by FernFlower decompiler)
//

package com.alibaba.fastjson.parser;

import com.alibaba.fastjson.JSONArray;
import com.alibaba.fastjson.JSONException;
import com.alibaba.fastjson.JSONObject;
import com.alibaba.fastjson.JSONPObject;
import com.alibaba.fastjson.JSONPath;
import com.alibaba.fastjson.PropertyNamingStrategy;
import com.alibaba.fastjson.annotation.JSONField;
import com.alibaba.fastjson.annotation.JSONType;
import com.alibaba.fastjson.parser.deserializer.ASMDeserializerFactory;
import com.alibaba.fastjson.parser.deserializer.ArrayListTypeFieldDeserializer;
import com.alibaba.fastjson.parser.deserializer.AutowiredObjectDeserializer;
import com.alibaba.fastjson.parser.deserializer.DefaultFieldDeserializer;
import com.alibaba.fastjson.parser.deserializer.EnumDeserializer;
import com.alibaba.fastjson.parser.deserializer.FieldDeserializer;
import com.alibaba.fastjson.parser.deserializer.JSONPDeserializer;
import com.alibaba.fastjson.parser.deserializer.JavaBeanDeserializer;
import com.alibaba.fastjson.parser.deserializer.JavaObjectDeserializer;
import com.alibaba.fastjson.parser.deserializer.Jdk8DateCodec;
import com.alibaba.fastjson.parser.deserializer.MapDeserializer;
import com.alibaba.fastjson.parser.deserializer.NumberDeserializer;
import com.alibaba.fastjson.parser.deserializer.ObjectDeserializer;
import com.alibaba.fastjson.parser.deserializer.OptionalCodec;
import com.alibaba.fastjson.parser.deserializer.SqlDateDeserializer;
import com.alibaba.fastjson.parser.deserializer.StackTraceElementDeserializer;
import com.alibaba.fastjson.parser.deserializer.ThrowableDeserializer;
import com.alibaba.fastjson.parser.deserializer.TimeDeserializer;
import com.alibaba.fastjson.serializer.AtomicCodec;
import com.alibaba.fastjson.serializer.AwtCodec;
import com.alibaba.fastjson.serializer.BigDecimalCodec;
import com.alibaba.fastjson.serializer.BigIntegerCodec;
import com.alibaba.fastjson.serializer.BooleanCodec;
import com.alibaba.fastjson.serializer.CalendarCodec;
import com.alibaba.fastjson.serializer.CharArrayCodec;
import com.alibaba.fastjson.serializer.CharacterCodec;
import com.alibaba.fastjson.serializer.CollectionCodec;
import com.alibaba.fastjson.serializer.DateCodec;
import com.alibaba.fastjson.serializer.FloatCodec;
import com.alibaba.fastjson.serializer.IntegerCodec;
import com.alibaba.fastjson.serializer.LongCodec;
import com.alibaba.fastjson.serializer.MiscCodec;
import com.alibaba.fastjson.serializer.ObjectArrayCodec;
import com.alibaba.fastjson.serializer.ReferenceCodec;
import com.alibaba.fastjson.serializer.StringCodec;
import com.alibaba.fastjson.util.ASMClassLoader;
import com.alibaba.fastjson.util.ASMUtils;
import com.alibaba.fastjson.util.FieldInfo;
import com.alibaba.fastjson.util.IOUtils;
import com.alibaba.fastjson.util.IdentityHashMap;
import com.alibaba.fastjson.util.JavaBeanInfo;
import com.alibaba.fastjson.util.ServiceLoader;
import com.alibaba.fastjson.util.TypeUtils;
import java.io.Closeable;
import java.io.File;
import java.io.Serializable;
import java.lang.ref.SoftReference;
import java.lang.ref.WeakReference;
import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.Modifier;
import java.lang.reflect.ParameterizedType;
import java.lang.reflect.Type;
import java.lang.reflect.TypeVariable;
import java.lang.reflect.WildcardType;
import java.math.BigDecimal;
import java.math.BigInteger;
import java.net.Inet4Address;
import java.net.Inet6Address;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.URI;
import java.net.URL;
import java.nio.charset.Charset;
import java.security.AccessControlException;
import java.sql.Date;
import java.sql.Time;
import java.sql.Timestamp;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Calendar;
import java.util.Collection;
import java.util.Currency;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Properties;
import java.util.Set;
import java.util.TimeZone;
import java.util.TreeMap;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicIntegerArray;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicLongArray;
import java.util.concurrent.atomic.AtomicReference;
import java.util.regex.Pattern;
import javax.sql.DataSource;
import javax.xml.datatype.XMLGregorianCalendar;

public class ParserConfig {
    public static final String DENY_PROPERTY = "fastjson.parser.deny";
    public static final String AUTOTYPE_ACCEPT = "fastjson.parser.autoTypeAccept";
    public static final String AUTOTYPE_SUPPORT_PROPERTY = "fastjson.parser.autoTypeSupport";
    public static final String[] DENYS;
    private static final String[] AUTO_TYPE_ACCEPT_LIST;
    public static final boolean AUTO_SUPPORT;
    public static ParserConfig global;
    private final IdentityHashMap<Type, ObjectDeserializer> deserializers;
    private boolean asmEnable;
    public final SymbolTable symbolTable;
    public PropertyNamingStrategy propertyNamingStrategy;
    protected ClassLoader defaultClassLoader;
    protected ASMDeserializerFactory asmFactory;
    private static boolean awtError;
    private static boolean jdk8Error;
    private boolean autoTypeSupport;
    private String[] denyList;
    private String[] acceptList;
    private int maxTypeNameLength;
    public final boolean fieldBased;
    public boolean compatibleWithJavaBean;

    public static ParserConfig getGlobalInstance() {
        return global;
    }

    public ParserConfig() {
        this(false);
    }

    public ParserConfig(boolean fieldBase) {
        this((ASMDeserializerFactory)null, (ClassLoader)null, fieldBase);
    }

    public ParserConfig(ClassLoader parentClassLoader) {
        this((ASMDeserializerFactory)null, parentClassLoader, false);
    }

    public ParserConfig(ASMDeserializerFactory asmFactory) {
        this(asmFactory, (ClassLoader)null, false);
    }

    private ParserConfig(ASMDeserializerFactory asmFactory, ClassLoader parentClassLoader, boolean fieldBased) {
        this.deserializers = new IdentityHashMap();
        this.asmEnable = !ASMUtils.IS_ANDROID;
        this.symbolTable = new SymbolTable(4096);
        this.autoTypeSupport = AUTO_SUPPORT;
        this.denyList = "bsh,com.mchange,com.sun.,java.lang.Thread,java.net.Socket,java.rmi,javax.xml,org.apache.bcel,org.apache.commons.beanutils,org.apache.commons.collections.Transformer,org.apache.commons.collections.functors,org.apache.commons.collections4.comparators,org.apache.commons.fileupload,org.apache.myfaces.context.servlet,org.apache.tomcat,org.apache.wicket.util,org.codehaus.groovy.runtime,org.hibernate,org.jboss,org.mozilla.javascript,org.python.core,org.springframework".split(",");
        this.acceptList = AUTO_TYPE_ACCEPT_LIST;
        this.maxTypeNameLength = 256;
        this.compatibleWithJavaBean = TypeUtils.compatibleWithJavaBean;
        this.fieldBased = fieldBased;
        if (asmFactory == null && !ASMUtils.IS_ANDROID) {
            try {
                if (parentClassLoader == null) {
                    asmFactory = new ASMDeserializerFactory(new ASMClassLoader());
                } else {
                    asmFactory = new ASMDeserializerFactory(parentClassLoader);
                }
            } catch (ExceptionInInitializerError var5) {
            } catch (AccessControlException var6) {
            } catch (NoClassDefFoundError var7) {
            }
        }

        this.asmFactory = asmFactory;
        if (asmFactory == null) {
            this.asmEnable = false;
        }

        this.deserializers.put(SimpleDateFormat.class, MiscCodec.instance);
        this.deserializers.put(Timestamp.class, SqlDateDeserializer.instance_timestamp);
        this.deserializers.put(Date.class, SqlDateDeserializer.instance);
        this.deserializers.put(Time.class, TimeDeserializer.instance);
        this.deserializers.put(java.util.Date.class, DateCodec.instance);
        this.deserializers.put(Calendar.class, CalendarCodec.instance);
        this.deserializers.put(XMLGregorianCalendar.class, CalendarCodec.instance);
        this.deserializers.put(JSONObject.class, MapDeserializer.instance);
        this.deserializers.put(JSONArray.class, CollectionCodec.instance);
        this.deserializers.put(Map.class, MapDeserializer.instance);
        this.deserializers.put(HashMap.class, MapDeserializer.instance);
        this.deserializers.put(LinkedHashMap.class, MapDeserializer.instance);
        this.deserializers.put(TreeMap.class, MapDeserializer.instance);
        this.deserializers.put(ConcurrentMap.class, MapDeserializer.instance);
        this.deserializers.put(ConcurrentHashMap.class, MapDeserializer.instance);
        this.deserializers.put(Collection.class, CollectionCodec.instance);
        this.deserializers.put(List.class, CollectionCodec.instance);
        this.deserializers.put(ArrayList.class, CollectionCodec.instance);
        this.deserializers.put(Object.class, JavaObjectDeserializer.instance);
        this.deserializers.put(String.class, StringCodec.instance);
        this.deserializers.put(StringBuffer.class, StringCodec.instance);
        this.deserializers.put(StringBuilder.class, StringCodec.instance);
        this.deserializers.put(Character.TYPE, CharacterCodec.instance);
        this.deserializers.put(Character.class, CharacterCodec.instance);
        this.deserializers.put(Byte.TYPE, NumberDeserializer.instance);
        this.deserializers.put(Byte.class, NumberDeserializer.instance);
        this.deserializers.put(Short.TYPE, NumberDeserializer.instance);
        this.deserializers.put(Short.class, NumberDeserializer.instance);
        this.deserializers.put(Integer.TYPE, IntegerCodec.instance);
        this.deserializers.put(Integer.class, IntegerCodec.instance);
        this.deserializers.put(Long.TYPE, LongCodec.instance);
        this.deserializers.put(Long.class, LongCodec.instance);
        this.deserializers.put(BigInteger.class, BigIntegerCodec.instance);
        this.deserializers.put(BigDecimal.class, BigDecimalCodec.instance);
        this.deserializers.put(Float.TYPE, FloatCodec.instance);
        this.deserializers.put(Float.class, FloatCodec.instance);
        this.deserializers.put(Double.TYPE, NumberDeserializer.instance);
        this.deserializers.put(Double.class, NumberDeserializer.instance);
        this.deserializers.put(Boolean.TYPE, BooleanCodec.instance);
        this.deserializers.put(Boolean.class, BooleanCodec.instance);
        this.deserializers.put(Class.class, MiscCodec.instance);
        this.deserializers.put(char[].class, new CharArrayCodec());
        this.deserializers.put(AtomicBoolean.class, BooleanCodec.instance);
        this.deserializers.put(AtomicInteger.class, IntegerCodec.instance);
        this.deserializers.put(AtomicLong.class, LongCodec.instance);
        this.deserializers.put(AtomicReference.class, ReferenceCodec.instance);
        this.deserializers.put(WeakReference.class, ReferenceCodec.instance);
        this.deserializers.put(SoftReference.class, ReferenceCodec.instance);
        this.deserializers.put(UUID.class, MiscCodec.instance);
        this.deserializers.put(TimeZone.class, MiscCodec.instance);
        this.deserializers.put(Locale.class, MiscCodec.instance);
        this.deserializers.put(Currency.class, MiscCodec.instance);
        this.deserializers.put(InetAddress.class, MiscCodec.instance);
        this.deserializers.put(Inet4Address.class, MiscCodec.instance);
        this.deserializers.put(Inet6Address.class, MiscCodec.instance);
        this.deserializers.put(InetSocketAddress.class, MiscCodec.instance);
        this.deserializers.put(File.class, MiscCodec.instance);
        this.deserializers.put(URI.class, MiscCodec.instance);
        this.deserializers.put(URL.class, MiscCodec.instance);
        this.deserializers.put(Pattern.class, MiscCodec.instance);
        this.deserializers.put(Charset.class, MiscCodec.instance);
        this.deserializers.put(JSONPath.class, MiscCodec.instance);
        this.deserializers.put(Number.class, NumberDeserializer.instance);
        this.deserializers.put(AtomicIntegerArray.class, AtomicCodec.instance);
        this.deserializers.put(AtomicLongArray.class, AtomicCodec.instance);
        this.deserializers.put(StackTraceElement.class, StackTraceElementDeserializer.instance);
        this.deserializers.put(Serializable.class, JavaObjectDeserializer.instance);
        this.deserializers.put(Cloneable.class, JavaObjectDeserializer.instance);
        this.deserializers.put(Comparable.class, JavaObjectDeserializer.instance);
        this.deserializers.put(Closeable.class, JavaObjectDeserializer.instance);
        this.deserializers.put(JSONPObject.class, new JSONPDeserializer());
        this.addItemsToDeny(DENYS);
        this.addItemsToAccept(AUTO_TYPE_ACCEPT_LIST);
    }

    private static String[] splitItemsFormProperty(String property) {
        return property != null && property.length() > 0 ? property.split(",") : null;
    }

    public void configFromPropety(Properties properties) {
        String property = properties.getProperty("fastjson.parser.deny");
        String[] items = splitItemsFormProperty(property);
        this.addItemsToDeny(items);
        property = properties.getProperty("fastjson.parser.autoTypeAccept");
        items = splitItemsFormProperty(property);
        this.addItemsToAccept(items);
        property = properties.getProperty("fastjson.parser.autoTypeSupport");
        if ("true".equals(property)) {
            this.autoTypeSupport = true;
        } else if ("false".equals(property)) {
            this.autoTypeSupport = false;
        }

    }

    private void addItemsToDeny(String[] items) {
        if (items != null) {
            for(int i = 0; i < items.length; ++i) {
                String item = items[i];
                this.addDeny(item);
            }

        }
    }

    private void addItemsToAccept(String[] items) {
        if (items != null) {
            for(int i = 0; i < items.length; ++i) {
                String item = items[i];
                this.addAccept(item);
            }

        }
    }

    public boolean isAutoTypeSupport() {
        return this.autoTypeSupport;
    }

    public void setAutoTypeSupport(boolean autoTypeSupport) {
        this.autoTypeSupport = autoTypeSupport;
    }

    public boolean isAsmEnable() {
        return this.asmEnable;
    }

    public void setAsmEnable(boolean asmEnable) {
        this.asmEnable = asmEnable;
    }

    public IdentityHashMap<Type, ObjectDeserializer> getDeserializers() {
        return this.deserializers;
    }

    public ObjectDeserializer getDeserializer(Type type) {
        ObjectDeserializer derializer = (ObjectDeserializer)this.deserializers.get(type);
        if (derializer != null) {
            return derializer;
        } else if (type instanceof Class) {
            return this.getDeserializer((Class)type, type);
        } else if (type instanceof ParameterizedType) {
            Type rawType = ((ParameterizedType)type).getRawType();
            return rawType instanceof Class ? this.getDeserializer((Class)rawType, type) : this.getDeserializer(rawType);
        } else {
            return JavaObjectDeserializer.instance;
        }
    }

    public ObjectDeserializer getDeserializer(Class<?> clazz, Type type) {
        ObjectDeserializer derializer = (ObjectDeserializer)this.deserializers.get(type);
        if (derializer != null) {
            return derializer;
        } else {
            if (type == null) {
                type = clazz;
            }

            ObjectDeserializer derializer = (ObjectDeserializer)this.deserializers.get(type);
            if (derializer != null) {
                return (ObjectDeserializer)derializer;
            } else {
                JSONType annotation = (JSONType)clazz.getAnnotation(JSONType.class);
                if (annotation != null) {
                    Class<?> mappingTo = annotation.mappingTo();
                    if (mappingTo != Void.class) {
                        return this.getDeserializer(mappingTo, mappingTo);
                    }
                }

                if (type instanceof WildcardType || type instanceof TypeVariable || type instanceof ParameterizedType) {
                    derializer = (ObjectDeserializer)this.deserializers.get(clazz);
                }

                if (derializer != null) {
                    return (ObjectDeserializer)derializer;
                } else {
                    String className = clazz.getName();
                    className = className.replace('$', '.');
                    if (className.startsWith("java.awt.") && AwtCodec.support(clazz) && !awtError) {
                        try {
                            this.deserializers.put(Class.forName("java.awt.Point"), AwtCodec.instance);
                            this.deserializers.put(Class.forName("java.awt.Font"), AwtCodec.instance);
                            this.deserializers.put(Class.forName("java.awt.Rectangle"), AwtCodec.instance);
                            this.deserializers.put(Class.forName("java.awt.Color"), AwtCodec.instance);
                        } catch (Throwable var11) {
                            awtError = true;
                        }

                        derializer = AwtCodec.instance;
                    }

                    if (!jdk8Error) {
                        try {
                            if (className.startsWith("java.time.")) {
                                this.deserializers.put(Class.forName("java.time.LocalDateTime"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.LocalDate"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.LocalTime"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.ZonedDateTime"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.OffsetDateTime"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.OffsetTime"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.ZoneOffset"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.ZoneRegion"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.ZoneId"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.Period"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.Duration"), Jdk8DateCodec.instance);
                                this.deserializers.put(Class.forName("java.time.Instant"), Jdk8DateCodec.instance);
                                derializer = (ObjectDeserializer)this.deserializers.get(clazz);
                            } else if (className.startsWith("java.util.Optional")) {
                                this.deserializers.put(Class.forName("java.util.Optional"), OptionalCodec.instance);
                                this.deserializers.put(Class.forName("java.util.OptionalDouble"), OptionalCodec.instance);
                                this.deserializers.put(Class.forName("java.util.OptionalInt"), OptionalCodec.instance);
                                this.deserializers.put(Class.forName("java.util.OptionalLong"), OptionalCodec.instance);
                                derializer = (ObjectDeserializer)this.deserializers.get(clazz);
                            }
                        } catch (Throwable var10) {
                            jdk8Error = true;
                        }
                    }

                    if (className.equals("java.nio.file.Path")) {
                        this.deserializers.put(clazz, MiscCodec.instance);
                    }

                    if (clazz == Map.Entry.class) {
                        this.deserializers.put(clazz, MiscCodec.instance);
                    }

                    ClassLoader classLoader = Thread.currentThread().getContextClassLoader();

                    try {
                        Iterator var6 = ServiceLoader.load(AutowiredObjectDeserializer.class, classLoader).iterator();

                        while(var6.hasNext()) {
                            AutowiredObjectDeserializer autowired = (AutowiredObjectDeserializer)var6.next();
                            Iterator var8 = autowired.getAutowiredFor().iterator();

                            while(var8.hasNext()) {
                                Type forType = (Type)var8.next();
                                this.deserializers.put(forType, autowired);
                            }
                        }
                    } catch (Exception var12) {
                    }

                    if (derializer == null) {
                        derializer = (ObjectDeserializer)this.deserializers.get(type);
                    }

                    if (derializer != null) {
                        return (ObjectDeserializer)derializer;
                    } else {
                        if (clazz.isEnum()) {
                            derializer = new EnumDeserializer(clazz);
                        } else if (clazz.isArray()) {
                            derializer = ObjectArrayCodec.instance;
                        } else if (clazz != Set.class && clazz != HashSet.class && clazz != Collection.class && clazz != List.class && clazz != ArrayList.class) {
                            if (Collection.class.isAssignableFrom(clazz)) {
                                derializer = CollectionCodec.instance;
                            } else if (Map.class.isAssignableFrom(clazz)) {
                                derializer = MapDeserializer.instance;
                            } else if (Throwable.class.isAssignableFrom(clazz)) {
                                derializer = new ThrowableDeserializer(this, clazz);
                            } else {
                                derializer = this.createJavaBeanDeserializer(clazz, (Type)type);
                            }
                        } else {
                            derializer = CollectionCodec.instance;
                        }

                        this.putDeserializer((Type)type, (ObjectDeserializer)derializer);
                        return (ObjectDeserializer)derializer;
                    }
                }
            }
        }
    }

    public void initJavaBeanDeserializers(Class<?>... classes) {
        if (classes != null) {
            Class[] var2 = classes;
            int var3 = classes.length;

            for(int var4 = 0; var4 < var3; ++var4) {
                Class<?> type = var2[var4];
                if (type != null) {
                    ObjectDeserializer deserializer = this.createJavaBeanDeserializer(type, type);
                    this.putDeserializer(type, deserializer);
                }
            }

        }
    }

    public ObjectDeserializer createJavaBeanDeserializer(Class<?> clazz, Type type) {
        boolean asmEnable = this.asmEnable & !this.fieldBased;
        if (asmEnable) {
            JSONType jsonType = (JSONType)clazz.getAnnotation(JSONType.class);
            Class superClass;
            if (jsonType != null) {
                superClass = jsonType.deserializer();
                if (superClass != Void.class) {
                    try {
                        Object deseralizer = superClass.newInstance();
                        if (deseralizer instanceof ObjectDeserializer) {
                            return (ObjectDeserializer)deseralizer;
                        }
                    } catch (Throwable var16) {
                    }
                }

                asmEnable = jsonType.asm();
            }

            if (asmEnable) {
                superClass = JavaBeanInfo.getBuilderClass(jsonType);
                if (superClass == null) {
                    superClass = clazz;
                }

                do {
                    if (!Modifier.isPublic(superClass.getModifiers())) {
                        asmEnable = false;
                        break;
                    }

                    superClass = superClass.getSuperclass();
                } while(superClass != Object.class && superClass != null);
            }
        }

        if (clazz.getTypeParameters().length != 0) {
            asmEnable = false;
        }

        if (asmEnable && this.asmFactory != null && this.asmFactory.classLoader.isExternalClass(clazz)) {
            asmEnable = false;
        }

        if (asmEnable) {
            asmEnable = ASMUtils.checkName(clazz.getSimpleName());
        }

        JavaBeanInfo beanInfo;
        if (asmEnable) {
            if (clazz.isInterface()) {
                asmEnable = false;
            }

            beanInfo = JavaBeanInfo.build(clazz, type, this.propertyNamingStrategy);
            if (asmEnable && beanInfo.fields.length > 200) {
                asmEnable = false;
            }

            Constructor<?> defaultConstructor = beanInfo.defaultConstructor;
            if (asmEnable && defaultConstructor == null && !clazz.isInterface()) {
                asmEnable = false;
            }

            FieldInfo[] var18 = beanInfo.fields;
            int var7 = var18.length;

            for(int var8 = 0; var8 < var7; ++var8) {
                FieldInfo fieldInfo = var18[var8];
                if (fieldInfo.getOnly) {
                    asmEnable = false;
                    break;
                }

                Class<?> fieldClass = fieldInfo.fieldClass;
                if (!Modifier.isPublic(fieldClass.getModifiers())) {
                    asmEnable = false;
                    break;
                }

                if (fieldClass.isMemberClass() && !Modifier.isStatic(fieldClass.getModifiers())) {
                    asmEnable = false;
                    break;
                }

                if (fieldInfo.getMember() != null && !ASMUtils.checkName(fieldInfo.getMember().getName())) {
                    asmEnable = false;
                    break;
                }

                JSONField annotation = fieldInfo.getAnnotation();
                if (annotation != null && (!ASMUtils.checkName(annotation.name()) || annotation.format().length() != 0 || annotation.deserializeUsing() != Void.class || annotation.unwrapped())) {
                    asmEnable = false;
                    break;
                }

                if (fieldClass.isEnum()) {
                    ObjectDeserializer fieldDeser = this.getDeserializer((Type)fieldClass);
                    if (!(fieldDeser instanceof EnumDeserializer)) {
                        asmEnable = false;
                        break;
                    }
                }
            }
        }

        if (asmEnable && clazz.isMemberClass() && !Modifier.isStatic(clazz.getModifiers())) {
            asmEnable = false;
        }

        if (!asmEnable) {
            return new JavaBeanDeserializer(this, clazz, type);
        } else {
            beanInfo = JavaBeanInfo.build(clazz, type, this.propertyNamingStrategy);

            try {
                return this.asmFactory.createJavaBeanDeserializer(this, beanInfo);
            } catch (NoSuchMethodException var13) {
                return new JavaBeanDeserializer(this, clazz, type);
            } catch (JSONException var14) {
                return new JavaBeanDeserializer(this, beanInfo);
            } catch (Exception var15) {
                throw new JSONException("create asm deserializer error, " + clazz.getName(), var15);
            }
        }
    }

    public FieldDeserializer createFieldDeserializer(ParserConfig mapping, JavaBeanInfo beanInfo, FieldInfo fieldInfo) {
        Class<?> clazz = beanInfo.clazz;
        Class<?> fieldClass = fieldInfo.fieldClass;
        Class<?> deserializeUsing = null;
        JSONField annotation = fieldInfo.getAnnotation();
        if (annotation != null) {
            deserializeUsing = annotation.deserializeUsing();
            if (deserializeUsing == Void.class) {
                deserializeUsing = null;
            }
        }

        return (FieldDeserializer)(deserializeUsing != null || fieldClass != List.class && fieldClass != ArrayList.class ? new DefaultFieldDeserializer(mapping, clazz, fieldInfo) : new ArrayListTypeFieldDeserializer(mapping, clazz, fieldInfo));
    }

    public void putDeserializer(Type type, ObjectDeserializer deserializer) {
        this.deserializers.put(type, deserializer);
    }

    public ObjectDeserializer getDeserializer(FieldInfo fieldInfo) {
        return this.getDeserializer(fieldInfo.fieldClass, fieldInfo.fieldType);
    }

    /** @deprecated */
    public boolean isPrimitive(Class<?> clazz) {
        return isPrimitive2(clazz);
    }

    /** @deprecated */
    public static boolean isPrimitive2(Class<?> clazz) {
        return clazz.isPrimitive() || clazz == Boolean.class || clazz == Character.class || clazz == Byte.class || clazz == Short.class || clazz == Integer.class || clazz == Long.class || clazz == Float.class || clazz == Double.class || clazz == BigInteger.class || clazz == BigDecimal.class || clazz == String.class || clazz == java.util.Date.class || clazz == Date.class || clazz == Time.class || clazz == Timestamp.class || clazz.isEnum();
    }

    public static void parserAllFieldToCache(Class<?> clazz, Map<String, Field> fieldCacheMap) {
        Field[] fields = clazz.getDeclaredFields();
        Field[] var3 = fields;
        int var4 = fields.length;

        for(int var5 = 0; var5 < var4; ++var5) {
            Field field = var3[var5];
            String fieldName = field.getName();
            if (!fieldCacheMap.containsKey(fieldName)) {
                fieldCacheMap.put(fieldName, field);
            }
        }

        if (clazz.getSuperclass() != null && clazz.getSuperclass() != Object.class) {
            parserAllFieldToCache(clazz.getSuperclass(), fieldCacheMap);
        }

    }

    public static Field getFieldFromCache(String fieldName, Map<String, Field> fieldCacheMap) {
        Field field = (Field)fieldCacheMap.get(fieldName);
        if (field == null) {
            field = (Field)fieldCacheMap.get("_" + fieldName);
        }

        if (field == null) {
            field = (Field)fieldCacheMap.get("m_" + fieldName);
        }

        if (field == null) {
            char c0 = fieldName.charAt(0);
            if (c0 >= 'a' && c0 <= 'z') {
                char[] chars = fieldName.toCharArray();
                chars[0] = (char)(chars[0] - 32);
                String fieldNameX = new String(chars);
                field = (Field)fieldCacheMap.get(fieldNameX);
            }
        }

        return field;
    }

    public ClassLoader getDefaultClassLoader() {
        return this.defaultClassLoader;
    }

    public void setDefaultClassLoader(ClassLoader defaultClassLoader) {
        this.defaultClassLoader = defaultClassLoader;
    }

    public void addDeny(String name) {
        if (name != null && name.length() != 0) {
            String[] denyList = this.denyList;
            int var3 = denyList.length;

            for(int var4 = 0; var4 < var3; ++var4) {
                String item = denyList[var4];
                if (name.equals(item)) {
                    return;
                }
            }

            denyList = new String[this.denyList.length + 1];
            System.arraycopy(this.denyList, 0, denyList, 0, this.denyList.length);
            denyList[denyList.length - 1] = name;
            this.denyList = denyList;
        }
    }

    public void addAccept(String name) {
        if (name != null && name.length() != 0) {
            String[] acceptList = this.acceptList;
            int var3 = acceptList.length;

            for(int var4 = 0; var4 < var3; ++var4) {
                String item = acceptList[var4];
                if (name.equals(item)) {
                    return;
                }
            }

            acceptList = new String[this.acceptList.length + 1];
            System.arraycopy(this.acceptList, 0, acceptList, 0, this.acceptList.length);
            acceptList[acceptList.length - 1] = name;
            this.acceptList = acceptList;
        }
    }

    public Class<?> checkAutoType(String typeName, Class<?> expectClass) {
        if (typeName == null) {
            return null;
        } else if (typeName.length() >= this.maxTypeNameLength) {
            throw new JSONException("autoType is not support. " + typeName);
        } else {
            String className = typeName.replace('$', '.');
            if (this.autoTypeSupport || expectClass != null) {
                int i;
                String deny;
                for(i = 0; i < this.acceptList.length; ++i) {
                    deny = this.acceptList[i];
                    if (className.startsWith(deny)) {
                        return TypeUtils.loadClass(typeName, this.defaultClassLoader);
                    }
                }

                for(i = 0; i < this.denyList.length; ++i) {
                    deny = this.denyList[i];
                    if (className.startsWith(deny)) {
                        throw new JSONException("autoType is not support. " + typeName);
                    }
                }
            }

            Class<?> clazz = TypeUtils.getClassFromMapping(typeName);
            if (clazz == null) {
                clazz = this.deserializers.findClass(typeName);
            }

            if (clazz != null) {
                if (expectClass != null && !expectClass.isAssignableFrom(clazz)) {
                    throw new JSONException("type not match. " + typeName + " -> " + expectClass.getName());
                } else {
                    return clazz;
                }
            } else {
                if (!this.autoTypeSupport) {
                    String accept;
                    int i;
                    for(i = 0; i < this.denyList.length; ++i) {
                        accept = this.denyList[i];
                        if (className.startsWith(accept)) {
                            throw new JSONException("autoType is not support. " + typeName);
                        }
                    }

                    for(i = 0; i < this.acceptList.length; ++i) {
                        accept = this.acceptList[i];
                        if (className.startsWith(accept)) {
                            clazz = TypeUtils.loadClass(typeName, this.defaultClassLoader);
                            if (expectClass != null && expectClass.isAssignableFrom(clazz)) {
                                throw new JSONException("type not match. " + typeName + " -> " + expectClass.getName());
                            }

                            return clazz;
                        }
                    }
                }

                if (this.autoTypeSupport || expectClass != null) {
                    clazz = TypeUtils.loadClass(typeName, this.defaultClassLoader);
                }

                if (clazz != null) {
                    if (ClassLoader.class.isAssignableFrom(clazz) || DataSource.class.isAssignableFrom(clazz)) {
                        throw new JSONException("autoType is not support. " + typeName);
                    }

                    if (expectClass != null) {
                        if (expectClass.isAssignableFrom(clazz)) {
                            return clazz;
                        }

                        throw new JSONException("type not match. " + typeName + " -> " + expectClass.getName());
                    }
                }

                if (!this.autoTypeSupport) {
                    throw new JSONException("autoType is not support. " + typeName);
                } else {
                    return clazz;
                }
            }
        }
    }

    static {
        String property = IOUtils.getStringProperty("fastjson.parser.deny");
        DENYS = splitItemsFormProperty(property);
        property = IOUtils.getStringProperty("fastjson.parser.autoTypeSupport");
        AUTO_SUPPORT = "true".equals(property);
        property = IOUtils.getStringProperty("fastjson.parser.autoTypeAccept");
        String[] items = splitItemsFormProperty(property);
        if (items == null) {
            items = new String[0];
        }

        AUTO_TYPE_ACCEPT_LIST = items;
        global = new ParserConfig();
        awtError = false;
        jdk8Error = false;
    }
}
