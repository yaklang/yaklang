package ssautil

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestForkedScope(t *testing.T) {
	table := NewRootVersionedTable[any]()
	table.CreateLexicalVariable("a", 1)

	// if true
	sub1_1 := table.CreateSubScope()
	sub1_1.CreateLexicalVariable("a", 2)

	// if false
	sub1_2 := table.CreateSubScope()
	sub1_2.CreateLexicalVariable("a", 3)

	test := assert.New(t)
	test.Equal(1, len(table.GetVersions("a")))
	test.Equal(2, len(sub1_2.GetVersions("a")))
	test.Equal(2, len(sub1_1.GetVersions("a")))

	test.Equal(1, table.GetLatestVersion("a").Value)
	test.Equal(2, sub1_1.GetLatestVersion("a").Value)
	test.Equal(3, sub1_2.GetLatestVersion("a").Value)

	// endif
	ProducePhi(func(a ...any) any {
		return 4
	}, sub1_1, sub1_2)
	test.Equal(4, table.GetLatestVersion("a").Value)
}

func TestMemberTrace(t *testing.T) {
	test := assert.New(t)

	table := NewRootVersionedTable[any]()
	var a = table.CreateLexicalVariable("a", nil)
	var b = table.CreateLexicalVariable("b", nil)
	err := table.RenameAssociated(b.GetId(), a.GetId())
	test.Nil(err)

	name1, err := table.ConvertStaticMemberCallToLexicalName(b.GetId(), "c")
	test.Nil(err)
	name2, err := table.ConvertStaticMemberCallToLexicalName(a.GetId(), "c")
	test.Nil(err)
	t.Log(name1, name2)
	test.Equal(name1, name2)

	table.CreateStaticMemberCallVariable(a.GetId(), "c", nil)
	table.CreateStaticMemberCallVariable(b.GetId(), "c", nil)

	test.Equal(2, len(table.GetVersions(name2)))
	test.Equal(2, len(table.GetVersions(name1)))

	sub := table.CreateSubScope()
	e := sub.CreateLexicalVariable("e", nil)
	err = sub.RenameAssociated(e.GetId(), a.GetId())
	test.Nil(err)

	eV, err := sub.CreateStaticMemberCallVariable(e.GetId(), "c", nil)
	test.Nil(err)

	t.Log(eV.String())

	test.Equal(3, len(sub.GetVersions(name2)))
	test.Equal(2, len(table.GetVersions(name2)))
	sub2 := table.CreateSubScope()
	test.Equal(2, len(sub2.GetVersions(name2)))
	sub2.CreateStaticMemberCallVariable(b.GetId(), "c", nil)
	test.Equal(3, len(sub2.GetVersions(name2)))
	sub2.CreateStaticMemberCallVariable(b.GetId(), "c", nil)
	test.Equal(4, len(sub2.GetVersions(name2)))

	ProducePhi(func(a ...any) any {
		return nil
	}, sub, sub2)

	l := table.GetLatestVersion(name2)
	t.Log(l.String())
	test.True(l.IsPhi())
	test.Equal(3, len(table.GetVersions(name1)))
}

func TestScopeAndPhi(t *testing.T) {
	test := assert.New(t)

	table := NewRootVersionedTable[any]()
	table.CreateLexicalVariable("a", nil)
	table.CreateLexicalVariable("a", nil)
	table.CreateLexicalVariable("ccc", nil)
	table.CreateLexicalVariable("a", nil)
	table.CreateLexicalVariable("a", nil)
	table.CreateLexicalVariable("a", nil)
	test.Equal(len(table.GetVersions("a")), 5)
	test.Equal(len(table.GetVersions("ccc")), 1)
	test.Equal(len(table.GetVersions("ddd")), 0)

	sub1 := table.CreateSubScope()
	test.Equal(len(sub1.GetVersions("a")), 5)
	v1 := sub1.CreateLexicalVariable("a", nil)
	_ = v1
	t.Log(v1.String())
	test.Equal(len(sub1.GetVersions("a")), 6)
	test.NotNil(sub1.GetLatestVersionInCurrentLexicalScope("a"))
	sub2 := sub1.CreateSubScope()
	test.Nil(sub2.GetLatestVersionInCurrentLexicalScope("a"))

	sub1_2 := table.CreateSubScope()
	test.Equal(5, len(sub1_2.GetVersions("a")))
	sub1_2.CreateLexicalVariable("a", nil)
	test.Equal(6, len(sub1_2.GetVersions("a")))

	sub1.CreateLexicalVariable("ccc", nil)
	test.Equal(2, len(sub1.GetAllCapturedVariableNames()))
	test.Equal(1, len(sub1_2.GetAllCapturedVariableNames()))

	test.Equal(sub1_2.GetAllCapturedVariableNames()[0], "a")
	test.Equal(sub1.GetAllCapturedVariableNames()[0], "a")
	test.Equal(sub1.GetAllCapturedVariableNames()[1], "ccc")

	ProducePhi(func(a ...any) any {
		return nil
	}, sub1, sub1_2)
	test.Equal(6, len(table.GetVersions("a")))
	var a = table.GetLatestVersion("a")
	t.Log(a.String())
	test.True(a.IsPhi())
}
