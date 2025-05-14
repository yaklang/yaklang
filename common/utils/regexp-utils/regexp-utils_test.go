package regexp_utils

import (
	"regexp"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
)

func TestYakRegexpUtilsOption(t *testing.T) {
	testRegexpOptionFunc := func(thisT *testing.T, regexpRaw, srcString, expected string, testOption regexp2.RegexOptions) {
		reUtils := NewYakRegexpUtils(regexpRaw, WithPriorityMode(RegexpMode1), WithRegexpOption(testOption))
		match, err := reUtils.MatchString(srcString)
		require.NoError(thisT, err)
		require.True(thisT, match)
		findString, err := reUtils.FindString(srcString)
		require.NoError(thisT, err)
		require.Equal(thisT, expected, findString)

		reUtils = NewYakRegexpUtils(regexpRaw, WithPriorityMode(RegexpMode1))
		match, err = reUtils.MatchString(srcString)
		require.NoError(thisT, err)
		require.False(thisT, match)
		findString, err = reUtils.FindString(srcString)
		require.NoError(thisT, err)
		require.Equal(thisT, "", findString)
	}

	t.Run("ignoreCase", func(t *testing.T) {
		testRegexpOptionFunc(t, "abc", "cccccccABC", "ABC", regexp2.IgnoreCase)
	})

	t.Run("singleline", func(t *testing.T) {
		testRegexpOptionFunc(t, "<div>.*</div>", "<div>\n<a>abc</a>\n</div>", "<div>\n<a>abc</a>\n</div>", regexp2.Singleline)
	})

	t.Run("multiline", func(t *testing.T) {
		srcString := "Joe 164\n" +
			"Sam 208\n" +
			"Allison 211\n" +
			"Gwen 171\n"
		testRegexpOptionFunc(t, "^(\\w+)\\s(\\d+)$", srcString, "Joe 164", regexp2.Multiline)

	})
}

func TestYakRegexpUtils_Priority(t *testing.T) {
	t.Run("re1 Compile fail", func(t *testing.T) {
		testRule := "cc(?#comment)abc"
		_, err := regexp.Compile(testRule)
		require.Error(t, err)
		reUtils := NewYakRegexpUtils(testRule, WithPriorityMode(RegexpMode1))
		match, err := reUtils.MatchString("ccabc")
		require.NoError(t, err)
		require.True(t, match)
	})

	t.Run("re2 not support named class", func(t *testing.T) {
		testRule := "[[:alpha:]]"
		reUtils := NewYakRegexpUtils(testRule, WithPriorityMode(RegexpMode2))
		match, err := reUtils.MatchString("ccabc")
		require.NoError(t, err)
		require.False(t, match)

		reUtils.SetPriority(RegexpMode1)
		match, err = reUtils.MatchString("ccabc")
		require.NoError(t, err)
		require.True(t, match)
	})
}

func TestSubmatchIndex(t *testing.T) {

	// reg := NewYakRegexpUtils(rule)
	// res, err := reg.FindAllSubmatchIndex(code)
	// require.NoError(t, err)
	// for i, v := range res {
	// 	log.Infof("submatch %d: %v", i, v)
	// 	if len(v) < 2 {
	// 		continue
	// 	}
	// 	log.Infof("submatch %d: %s", i, code[v[0]:v[1]])
	// }

	check := func(t *testing.T, code, rule string, want []string) {
		reg := NewYakRegexpUtils(rule)
		res, err := reg.FindAllSubmatchIndex(code)
		require.NoError(t, err)
		got := make([]string, 0, len(res))
		for _, v := range res {
			if len(v) < 2 {
				continue
			}
			got = append(got, code[v[0]:v[1]])
		}
		log.Infof("got: %v", got)
		require.NotEqual(t, len(got), 0)
		// each element of want should exist in got
		for _, want := range want {
			require.Contains(t, got, want)
		}
	}

	t.Run("check regexp1", func(t *testing.T) {

		code := `
	spring.datasource.url=jdbc:mysql://localhost:3306/your_database
	spring.datasource.username=your_username
	spring.datasource.password=your_password
	spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
	spring.jpa.hibernate.ddl-auto=update
	spring.jpa.properties.hibernate.dialect=org.hibernate.dialect.MySQL5InnoDBDialect
	`

		rule := `spring.datasource.url=(.*)`

		check(t, code, rule, []string{
			"spring.datasource.url=jdbc:mysql://localhost:3306/your_database",
			"jdbc:mysql://localhost:3306/your_database",
		})
	})

	t.Run("check regexp2", func(t *testing.T) {
		code := `
### check referer configuration begins ###
joychou.security.referer.enabled = false
joychou.security.referer.host = joychou.org, joychou.com
# Only support ant url style.
joychou.security.referer.uri = /jsonp/**
### check referer configuration ends ###
# Fake aksk. Simulate actuator info leak.
jsc.accessKey.id=aaaaaaaaaaaa
jsc.accessKey.secret=bbbbbbbbbbbbbbbbb
		`

		rule := `(?i).*access[_-]?[token|key].*\s*=\s*((?!\{\{)(?!(?i)^(true|false|on|off|yes|no|y|n|null)).+)`

		check(t, code, rule, []string{
			"jsc.accessKey.id=aaaaaaaaaaaa",
			"jsc.accessKey.secret=bbbbbbbbbbbbbbbbb",
		})
	})
}
