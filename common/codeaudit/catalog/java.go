package catalog

// FrameworkSignal describes detection signals for a framework.
type FrameworkSignal struct {
	Name                 string   // e.g. "spring_boot"
	Display              string   // e.g. "Spring Boot / Spring MVC"
	FileMarkers          []string // file names whose presence indicates the framework
	ContentMarkers       []string // content patterns in matching files
	StrongContentMarkers []string // strong content markers (extra confidence)
	ArchTool             string   // recommended arch info tool name
	ConfigTool           string   // recommended config audit tool name
}

// JavaFrameworkCatalog contains detection signals for 12 mainstream Java frameworks.
var JavaFrameworkCatalog = []FrameworkSignal{
	{
		Name:                 "spring_boot",
		Display:              "Spring Boot / Spring MVC",
		FileMarkers:          []string{"pom.xml", "build.gradle", "application.properties", "application.yml", "application.yaml"},
		ContentMarkers:       []string{"@SpringBootApplication", "org.springframework.boot", "spring-boot-starter"},
		StrongContentMarkers: []string{"@SpringBootApplication", "spring-boot-starter", "org.springframework.boot"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "servlet",
		Display:              "Servlet / Java EE",
		FileMarkers:          []string{"pom.xml", "build.gradle", "web.xml"},
		ContentMarkers:       []string{"javax.servlet", "jakarta.servlet", "HttpServlet"},
		StrongContentMarkers: []string{"javax.servlet.http.HttpServlet", "jakarta.servlet.http.HttpServlet"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "struts2",
		Display:              "Apache Struts 2",
		FileMarkers:          []string{"struts.xml", "struts.properties", "pom.xml", "build.gradle"},
		ContentMarkers:       []string{"org.apache.struts2", "com.opensymphony.xwork2", "struts2-core"},
		StrongContentMarkers: []string{"struts2-core", "org.apache.struts2"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "mybatis",
		Display:              "MyBatis",
		FileMarkers:          []string{"pom.xml", "build.gradle"},
		ContentMarkers:       []string{"org.mybatis", "mybatis-spring", "mybatis-generator"},
		StrongContentMarkers: []string{"mybatis-spring-boot-starter", "org.mybatis.spring"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "shiro",
		Display:              "Apache Shiro",
		FileMarkers:          []string{"shiro.ini", "pom.xml", "build.gradle"},
		ContentMarkers:       []string{"org.apache.shiro", "shiro-core", "shiro-spring"},
		StrongContentMarkers: []string{"shiro-core", "org.apache.shiro.web"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "spring_security",
		Display:              "Spring Security",
		FileMarkers:          []string{"pom.xml", "build.gradle"},
		ContentMarkers:       []string{"org.springframework.security", "spring-security-config", "spring-security-web"},
		StrongContentMarkers: []string{"spring-security-config", "org.springframework.security.config"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "jpa",
		Display:              "Hibernate / JPA",
		FileMarkers:          []string{"pom.xml", "build.gradle", "persistence.xml"},
		ContentMarkers:       []string{"javax.persistence", "jakarta.persistence", "org.hibernate", "hibernate-core"},
		StrongContentMarkers: []string{"hibernate-core", "org.hibernate.SessionFactory"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "dubbo",
		Display:              "Apache Dubbo",
		FileMarkers:          []string{"pom.xml", "build.gradle", "dubbo.xml"},
		ContentMarkers:       []string{"org.apache.dubbo", "com.alibaba.dubbo", "dubbo-spring-boot-starter"},
		StrongContentMarkers: []string{"dubbo-spring-boot-starter", "org.apache.dubbo.config"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "spring_cloud",
		Display:              "Spring Cloud",
		FileMarkers:          []string{"pom.xml", "build.gradle", "bootstrap.yml", "bootstrap.properties"},
		ContentMarkers:       []string{"org.springframework.cloud", "spring-cloud-starter", "spring-cloud-dependencies"},
		StrongContentMarkers: []string{"spring-cloud-starter", "org.springframework.cloud"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "jfinal",
		Display:              "JFinal",
		FileMarkers:          []string{"pom.xml", "build.gradle"},
		ContentMarkers:       []string{"com.jfinal", "jfinal-java"},
		StrongContentMarkers: []string{"com.jfinal.core", "jfinal-java"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "vertx",
		Display:              "Eclipse Vert.x",
		FileMarkers:          []string{"pom.xml", "build.gradle", "vertx-default-jul-logging.properties"},
		ContentMarkers:       []string{"io.vertx", "vertx-core", "vertx-web"},
		StrongContentMarkers: []string{"vertx-core", "io.vertx.core"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
	{
		Name:                 "play",
		Display:              "Play Framework",
		FileMarkers:          []string{"build.gradle", "build.sbt", "application.conf", "routes"},
		ContentMarkers:       []string{"play.api", "play.mvc", "com.typesafe.play"},
		StrongContentMarkers: []string{"com.typesafe.play", "play.mvc.Controller"},
		ArchTool:             "java_framework_arch_info",
		ConfigTool:           "java_framework_config_audit",
	},
}

// CmsFingerprint describes a fingerprint for a Java CMS / backend product.
type CmsFingerprint struct {
	ID             string
	Display        string
	Family         string
	FileMarkers    []string
	ContentMarkers []string // supports regex
	MinScore       float64  // 0 = use global default
}

// JavaCmsCatalog contains fingerprints for 8 common Java CMS products.
var JavaCmsCatalog = []CmsFingerprint{
	{
		ID:             "ruoyi",
		Display:        "RuoYi 若依",
		Family:         "ruoyi",
		FileMarkers:    []string{"ruoyi-admin", "ruoyi-common", "ruoyi-framework", "RuoYiApplication.java"},
		ContentMarkers: []string{`com\.ruoyi`, "RuoYiApplication", `artifactId>ruoyi`},
		MinScore:       0,
	},
	{
		ID:             "ruoyi-cloud",
		Display:        "RuoYi-Cloud 若依微服务",
		Family:         "ruoyi",
		FileMarkers:    []string{"ruoyi-modules", "ruoyi-gateway", "ruoyi-auth", "ruoyi-api"},
		ContentMarkers: []string{`com\.ruoyi`, "RuoYiCloudApplication", `artifactId>ruoyi-cloud`},
		MinScore:       0,
	},
	{
		ID:             "mcms",
		Display:        "铭飞 MCMS",
		Family:         "mingsoft",
		FileMarkers:    []string{"mcms", "mcms-web", "ms-mcms"},
		ContentMarkers: []string{`com\.mingsoft`, "net.mingsoft", "mcms"},
		MinScore:       0,
	},
	{
		ID:             "halo",
		Display:        "Halo CMS",
		Family:         "halo",
		FileMarkers:    []string{"halo", "halo-server", "application.yaml"},
		ContentMarkers: []string{`run\.halo\.app`, "halo-core", "HaloApplication"},
		MinScore:       0.50,
	},
	{
		ID:             "publiccms",
		Display:        "PublicCMS",
		Family:         "publiccms",
		FileMarkers:    []string{"publiccms", "PublicCMS", "cms.web"},
		ContentMarkers: []string{`com\.publiccms`, "publiccms", "CmsApplication"},
		MinScore:       0,
	},
	{
		ID:             "lin-cms",
		Display:        "Lin CMS",
		Family:         "lin-cms",
		FileMarkers:    []string{"lin-cms", "lincms", "lin-server"},
		ContentMarkers: []string{`io\.github\.linpeilin`, "lin-cms-spring-boot-starter", "LinCMS"},
		MinScore:       0,
	},
	{
		ID:             "mall",
		Display:        "mall 电商后台",
		Family:         "macrozheng",
		FileMarkers:    []string{"mall-admin", "mall-portal", "mall-search", "mall-mbg"},
		ContentMarkers: []string{`com\.macro\.mall`, "mall-common", "MallApplication"},
		MinScore:       0,
	},
	{
		ID:             "ofbiz",
		Display:        "Apache OFBiz",
		Family:         "ofbiz",
		FileMarkers:    []string{"ofbiz", "framework", "specialpurpose", "applications"},
		ContentMarkers: []string{`org\.apache\.ofbiz`, "ofbiz-framework", "ComponentConfig"},
		MinScore:       0.50,
	},
}

// ModeThresholds defines confidence thresholds for each detection mode.
var ModeThresholds = map[string]float64{
	"permissive": 0.25,
	"balanced":   0.35,
	"strict":     0.60,
}

// GetModeThreshold returns the confidence threshold for a detection mode.
func GetModeThreshold(mode string) float64 {
	if t, ok := ModeThresholds[mode]; ok {
		return t
	}
	return ModeThresholds["balanced"]
}
