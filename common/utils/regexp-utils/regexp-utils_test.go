package regexp_utils

import (
	"github.com/dlclark/regexp2"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
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
