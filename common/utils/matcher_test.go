package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMatchAllOfGlob(t *testing.T) {
	test := assert.New(t)
	test.True(MatchAllOfGlob("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "*rememberMe=*", "*=deleteMe;*"))
	test.False(MatchAllOfGlob("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "*rememberMe=*", "*=deleteMe;*", "asdfasdfasdfiuo32i4902345"))
	test.True(MatchAnyOfGlob("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=*", "*=deleteMe;"))
	test.True(MatchAnyOfGlob("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=*", "*=deleteMe;", "asdfasdfasdfiuo32i4902345"))
	test.True(MatchAnyOfGlob("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=23uejiasdfjopasdfkop", "*=deleteMe;", "asdfasdfasdfiuo32i4902345"))
	test.False(MatchAnyOfGlob("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=23uejiasdfjopasdfkop", "1111eteMe;", "asdfasdfasdfiuo32i4902345"))

	test.True(MatchAllOfRegexp("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=.*", ".*=deleteMe;"))
	test.False(MatchAllOfRegexp("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=.*", ".*=deleteMe;", "asdfhasdfjklajskldfjopasdfop"))
	test.True(MatchAnyOfRegexp("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=*", "*=deleteMe;"))
	test.True(MatchAnyOfRegexp("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=*", "*=deleteMe;"))
	test.False(MatchAnyOfRegexp("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMeahsiodjkoasdfjklasdfk=*", "*=deletasdnklfhjnklasdfjklasdfeMe;"))

	test.False(MatchAllOfSubString("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=.*", ".*=deleteMe;"))
	test.True(MatchAllOfSubString("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=", "=deleteMe;"))
	test.False(MatchAllOfSubString("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=.*", ".*=deleteMe;", "asdfhasdfjklajskldfjopasdfop"))
	test.False(MatchAllOfSubString("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=.*", "=deleteMe;", "asdfhasdfjklajskldfjopasdfop"))
	test.True(MatchAnyOfSubString("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=*", "=deleteMe;"))
	test.False(MatchAnyOfSubString("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMe=*", "*=deleteMe;"))
	test.False(MatchAnyOfSubString("asdfGET/HTTP Set-Cookie: rememberMe=deleteMe;", "rememberMeahsiodjkoasdfjklasdfk=*", "*=deletasdnklfhjnklasdfjklasdfeMe;"))
}
