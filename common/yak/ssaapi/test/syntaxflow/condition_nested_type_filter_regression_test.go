package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_NestedTypeFilterRegression(t *testing.T) {
	code := `
package com.example;

import javax.naming.Context;
import javax.naming.directory.DirContext;
import javax.naming.directory.InitialDirContext;
import javax.naming.directory.SearchControls;
import java.util.Hashtable;

public class LdapSearchDemo {
    public void search(String username) throws Exception {
        Hashtable<String, String> env = new Hashtable<>();
        env.put(Context.INITIAL_CONTEXT_FACTORY, "com.sun.jndi.ldap.LdapCtxFactory");
        DirContext ctx = new InitialDirContext(env);
        SearchControls searchCtls = new SearchControls();
        String searchFilter = "(&(objectClass=user)(sAMAccountName=" + username + "))";
        ctx.search("dc=example,dc=com", searchFilter, searchCtls);
    }
}
`

	tests := []struct {
		name string
		rule string
		want []string
	}{
		{
			name: "nested_type_filter_single_have_string",
			rule: `InitialDirContext()?{<typeName>?{have:'javax.naming'}}.search(*?{<typeName>?{have:'string'}} as $sink);`,
			want: []string{"dc=example,dc=com", "sAMAccountName"},
		},
		{
			name: "nested_type_filter_or_shortcut_const_branch",
			rule: `InitialDirContext()?{<typeName>?{have:'javax.naming'}}.search(*?{<typeName>?{have:'String'||'string'}} as $sink);`,
			want: []string{"dc=example,dc=com", "sAMAccountName"},
		},
		{
			name: "nested_type_filter_or_explicit_have_branch",
			rule: `InitialDirContext()?{<typeName>?{have:'javax.naming'}}.search(*?{<typeName>?{have:'String' || have:'string'}} as $sink);`,
			want: []string{"dc=example,dc=com", "sAMAccountName"},
		},
		{
			name: "nested_type_filter_and_or_inside_star_question",
			rule: `InitialDirContext()?{<typeName>?{have:'javax.naming'}}.search(*?{<typeName>?{have:'String'||'string'} && (have:'dc=example,dc=com' || have:'sAMAccountName')} as $sink);`,
			want: []string{"dc=example,dc=com", "sAMAccountName"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ssatest.CheckSyntaxFlowContain(t, code, tt.rule, map[string][]string{
				"sink": tt.want,
			}, ssaapi.WithLanguage(ssaconfig.JAVA))
		})
	}
}
