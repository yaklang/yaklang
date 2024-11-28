package example

import (
	"fmt"
	"log"
	"net/http"

	ldap "gopkg.in/ldap.v2"
)

func ldapSearch(username string) ([]string, error) {
	l, err := ldap.Dial("tcp", "ldap.example.com:389")
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// 绑定（使用一个固定的管理员用户）
	err = l.Bind("cn=admin,dc=example,dc=com", "password")
	if err != nil {
		return nil, err
	}

	// LDAP 查询，存在注入风险
	searchRequest := ldap.NewSearchRequest(
		"dc=example,dc=com",
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(uid=%s)", username), // 漏洞：直接插入用户输入
		[]string{"dn", "cn", "mail"},
		nil,
	)

	searchResult, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, entry := range searchResult.Entries {
		results = append(results, entry.GetAttributeValue("cn"))
	}
	return results, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	results, err := ldapSearch(username)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Results: %v\n", results)
}

func main() {
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
