package webfingerprint

var (
	DefaultWebFingerprintRules = []*WebRule{
		// phpmyadmin 的指纹比较特殊，需要先识别出是 phpmyadmin 才能进一步确定版本
		{
			Methods: []*WebMatcherMethods{
				{
					Keywords: []*KeywordMatcher{
						{
							CPE:    CPE{Product: "phpmyadmin", Vendor: "phpmyadmin"},
							Regexp: `<h.>Welcome to.*phpMyAdmin`,
						},
					},
				},
				{
					Keywords: []*KeywordMatcher{
						{
							CPE:    CPE{Product: "phpmyadmin", Vendor: "phpmyadmin"},
							Regexp: `phpmyadmin\.css\.php\?`,
						},
					},
				},
			},
			NextStep: &WebRule{
				Path: "/README",
				Methods: []*WebMatcherMethods{
					{Keywords: []*KeywordMatcher{
						{
							CPE:          CPE{Product: "phpmyadmin", Vendor: "phpmyadmin"},
							Regexp:       `Version (?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
							VersionIndex: 1,
						},
					}},
				},
			},
		},

		{
			Methods: []*WebMatcherMethods{
				{
					MD5s: []*MD5Matcher{
						{
							CPE: CPE{
								Vendor:  "oracle",
								Product: "weblogic_server",
							},
							MD5: "1af585e6c8cc77a6ca1832b608fd20aa"},
					},
				},
			},
			NextStep: &WebRule{
				Path: "/console/login/LoginForm.jsp",
				Methods: []*WebMatcherMethods{{
					Keywords: []*KeywordMatcher{{
						CPE:          CPE{Product: "weblogic", Vendor: "oracle"},
						Regexp:       `WebLogic Server.*: ([0-9\.]+)</p>`,
						VersionIndex: 1,
					}, {
						CPE:          CPE{Product: "weblogic_server", Vendor: "oracle"},
						Regexp:       `WebLogic Server.*: ([0-9\.]+)</p>`,
						VersionIndex: 1,
					}},
				}},
			},
		},

		// 无特殊路径以及无特殊操作的指纹识别
		{
			Methods: []*WebMatcherMethods{
				{
					HTTPHeaders: []*HTTPHeaderMatcher{
						// 通用匹配规则
						{
							HeaderName: "Server",
							HeaderValue: KeywordMatcher{
								Regexp:       `(?P<product>[a-zA-Z_0-9]+)/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								ProductIndex: 1,
								VersionIndex: 2,
							},
						},
						{
							HeaderName: "X-Powered-By",
							HeaderValue: KeywordMatcher{
								Regexp:       `(?P<product>[a-zA-Z_0-9]+)/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								ProductIndex: 1,
								VersionIndex: 2,
							},
						},

						{
							HeaderName: "X-Powered-By",
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "jsp", Vendor: "oracle"},
								Regexp:       `JSP/((\d+\.?)+)`,
								VersionIndex: 1,
							},
						},

						// IIS
						{
							HeaderName: "Server",
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "iis"},
								Regexp:       `Microsoft-IIS/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// glassfish_server
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "glassfish_server"},
								Regexp:       `Oracle GlassFish Server +(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "glassfish_server"},
								Regexp:       `GlassFish Server Open Source Edition +(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "glassfish_server"},
								Regexp:       `.*GlassFish Server Open Source Edition +(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// PHP
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "php"},
								Regexp:       `PHP/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// OpenSSL
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "openssl"},
								Regexp:       `OpenSSL/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9]+)?)`,
								VersionIndex: 1,
							},
						},

						// mod_fcgid
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: `mod_fcgid`},
								Regexp:       `mod_fcgid/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// mod_ssl
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: `mod_ssl`},
								Regexp:       `mod_ssl/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// weblogic_server
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "weblogic_server"},
								Regexp:       `WebLogic Server +(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?) (SP\d+)`,
								VersionIndex: 1,
								UpdateIndex:  5,
							},
						},

						// nginx
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "nginx"},
								Regexp:       `nginx/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "nginx"},
								Regexp:       `Nginx/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// jenkins
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Vendor: "eclipse", Product: "jetty"},
								Regexp:       `Jetty\((?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)\)`,
								VersionIndex: 1,
							},
						},

						// medusa
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "medusa"},
								Regexp:       `Medusa/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// oracle:application_server
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "application_server", Vendor: "oracle"},
								Regexp:       `Oracle-Application-Server-(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)(g|c|i)`,
								VersionIndex: 1,
							},
						},

						// oracle:http_server
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "http_server", Vendor: "oracle"},
								Regexp:       `Oracle-Application-Server-\d+g/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?) Oracle-HTTP-Server`,
								VersionIndex: 1,
							},
						},

						// oracle:web_cache
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "web_cache", Vendor: "oracle"},
								Regexp:       `Oracle-Web-Cache-\d+g/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "web_cache", Vendor: "oracle"},
								Regexp:       `OracleAS-Web-Cache-\d+g/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},

						// apache:http_server
						{
							HeaderValue: KeywordMatcher{
								CPE:          CPE{Product: "http_server", Vendor: "apache"},
								Regexp:       `Apache/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)`,
								VersionIndex: 1,
							},
						},
					},
					Keywords: []*KeywordMatcher{
						// Tomcat
						{
							Regexp:       `<title>Apache Tomcat/(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)</title>`,
							CPE:          CPE{Vendor: "apache", Product: "tomcat"},
							VersionIndex: 1,
						},

						// supervisord
						{
							CPE:          CPE{Product: "supervisord"},
							Regexp:       `<a href="http://supervisord.org">Supervisor</a> <span>(?P<version>((\d+\.?)+)?([a-zA-Z_0-9-]+)?)</span>`,
							VersionIndex: 1,
						},

						// wordpress
						{
							CPE:          CPE{Product: "wordpress", Vendor: "wordpress"},
							Regexp:       `< *meta[^>]*name *= *['"]generator['"][^>]*content *= *['"]WordPress ?([\d.]+)?`,
							VersionIndex: 1,
						},

						// jquery
						{
							CPE:          CPE{Product: "jquery", Vendor: "jquery"},
							Regexp:       `< *script[^>]*src *= *['"][^'"]*jquery-([0-9\.]+)(\.min)?\.js`,
							VersionIndex: 1,
						},
						{
							CPE:          CPE{Product: "jquery", Vendor: "jquery"},
							Regexp:       `< *script[^>]*src *= *['"][^'"]*jquery(\.min)?\.js\?ver=([0-9\.]+)`,
							VersionIndex: 2,
						},
						{
							CPE:          CPE{Product: "jquery", Vendor: "jquery"},
							Regexp:       `< *script[^>]*src *= *['"][^'"]*jquery\/([0-9\.]+)/jquery(\.min)?\.js`,
							VersionIndex: 1,
						},
					},
				},
			},
		},
	}
)
