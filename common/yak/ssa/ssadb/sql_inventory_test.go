package ssadb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const sqlInventoryPath = "testdata/sql_inventory.json"

// sqlInventoryEntry pins one SQL-containing string literal to the production
// file and function that owns it. A new or edited SQL literal changes the
// fingerprint and fails TestSQLInventory until the manifest is regenerated and
// reviewed.
type sqlInventoryEntry struct {
	File        string `json:"file"`
	Function    string `json:"function"`
	Fingerprint string `json:"fingerprint"`
	Snippet     string `json:"snippet"`
}

// TestSQLInventory is the static half of the SQL coverage gate. It cannot
// execute a new statement, but it makes every new SQL literal visible in the
// PR diff instead of letting it enter the package silently. Execution across
// SQLite/PostgreSQL/MySQL stays in TestSSADialectConformance.
func TestSQLInventory(t *testing.T) {
	entries, err := collectSQLInventory()
	if err != nil {
		t.Fatalf("collect SQL inventory: %v", err)
	}
	if os.Getenv("UPDATE_SQL_INVENTORY") == "1" {
		if err := writeSQLInventory(entries); err != nil {
			t.Fatalf("write SQL inventory: %v", err)
		}
		t.Logf("wrote %d SQL entries to %s", len(entries), sqlInventoryPath)
		return
	}

	want, err := readSQLInventory()
	if err != nil {
		t.Fatalf("read %s: %v", sqlInventoryPath, err)
	}
	added, removed := diffSQLInventory(want, entries)
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("SQL inventory changed.\n")
	for _, entry := range added {
		b.WriteString(fmt.Sprintf("  + %s %s %s\n", entry.File, entry.Function, entry.Snippet))
	}
	for _, entry := range removed {
		b.WriteString(fmt.Sprintf("  - %s %s %s\n", entry.File, entry.Function, entry.Snippet))
	}
	b.WriteString("\nReview every addition, make sure a conformance/unit test executes it, then run:\n")
	b.WriteString("  UPDATE_SQL_INVENTORY=1 go test ./common/yak/ssa/ssadb -run '^TestSQLInventory$' -count=1\n")
	t.Fatal(b.String())
}

func collectSQLInventory() ([]sqlInventoryEntry, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)

	seen := map[string]sqlInventoryEntry{}
	for _, name := range files {
		fileEntries, err := collectFileSQLInventory(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, entry := range fileEntries {
			seen[sqlInventoryKey(entry)] = entry
		}
	}
	out := make([]sqlInventoryEntry, 0, len(seen))
	for _, entry := range seen {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return sqlInventoryKey(out[i]) < sqlInventoryKey(out[j])
	})
	return out, nil
}

func collectFileSQLInventory(filename string) ([]sqlInventoryEntry, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}
	var entries []sqlInventoryEntry
	add := func(function string, literal *ast.BasicLit) {
		if literal.Kind != token.STRING {
			return
		}
		text, err := strconv.Unquote(literal.Value)
		if err != nil || !isSQLText(text) {
			return
		}
		normalized := normalizeSQLText(text)
		sum := sha256.Sum256([]byte(normalized))
		entries = append(entries, sqlInventoryEntry{
			File:        filepath.ToSlash(filename),
			Function:    function,
			Fingerprint: hex.EncodeToString(sum[:]),
			Snippet:     sqlSnippet(normalized),
		})
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body == nil {
				continue
			}
			ast.Inspect(d.Body, func(node ast.Node) bool {
				if lit, ok := node.(*ast.BasicLit); ok {
					add(d.Name.Name, lit)
				}
				return true
			})
		case *ast.GenDecl:
			ast.Inspect(d, func(node ast.Node) bool {
				if lit, ok := node.(*ast.BasicLit); ok {
					add("<package>", lit)
				}
				return true
			})
		}
	}
	return entries, nil
}

// isSQLText deliberately accepts both whole statements and clauses such as
// `" WHERE program_name = ?"`; SSA builds most native SQL by concatenation.
// Bare keywords and log messages containing words like "delete" or "index" are
// rejected by requiring SQL structure (FROM/SET/ON/punctuation).
func isSQLText(text string) bool {
	normalized := normalizeSQLText(text)
	// Log lines and error wrappers can contain SQL words such as "index" or
	// "table"; they are not statements. SQL format templates in this package
	// use %s/%d, not %w/%v.
	if strings.HasPrefix(normalized, "[") || strings.ContainsAny(normalized, "\n") {
		return false
	}
	for _, verb := range []string{"%w", "%v", "%q", "%!"} {
		if strings.Contains(normalized, verb) {
			return false
		}
	}
	upper := strings.ToUpper(normalized)
	switch {
	case strings.HasPrefix(upper, "SELECT "):
		return strings.Contains(upper, " FROM") || strings.ContainsAny(upper, ",?=*")
	case hasSQLPrefix(upper, text, "DELETE FROM "):
		return true
	case hasSQLPrefix(upper, text, "INSERT INTO "):
		return true
	case upper == "UPDATE":
		return strings.HasSuffix(text, " ")
	case strings.HasPrefix(upper, "UPDATE "):
		return strings.Contains(upper, " SET ") || strings.Contains(upper, "=")
	case hasSQLPrefix(upper, text, "CREATE "):
		return true
	case hasSQLPrefix(upper, text, "ALTER TABLE "):
		return true
	case hasSQLPrefix(upper, text, "DROP INDEX "), hasSQLPrefix(upper, text, "DROP TABLE "):
		return true
	case strings.HasPrefix(upper, "PRAGMA "):
		return true
	case strings.HasPrefix(upper, "WITH "):
		return strings.Contains(upper, " SELECT") || strings.ContainsAny(upper, ",?=(")
	case hasSQLPrefix(upper, text, "MERGE INTO "):
		return true
	}

	// Clause fragments only count when they also contain SQL punctuation. A
	// plain log message such as "loaded from database, hash=%s" stays out;
	// real SQL format templates start with a statement keyword and are handled
	// above before reaching this fallback.
	if strings.Contains(normalized, "%s") || strings.Contains(normalized, "%d") {
		return false
	}
	hasClause := false
	for _, clause := range []string{
		" FROM ", " WHERE ", " JOIN ", " SET ", " VALUES ", " GROUP BY ",
		" ORDER BY ", " LIMIT ", " INTO ", " INDEX ", " TABLE ",
		" COALESCE(", " REGEXP", " SYS_CONTEXT(", " NOT INDEXED", " INDEXED BY ",
	} {
		if strings.Contains(" "+upper+" ", clause) {
			hasClause = true
			break
		}
	}
	if !hasClause {
		return false
	}
	for _, token := range []string{"?", "=", "(", ")", ",", `"`, "`"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func hasSQLPrefix(upper, raw, prefix string) bool {
	trimmed := strings.TrimSpace(prefix)
	if upper == trimmed {
		// A bare keyword is SQL only when the source literal kept its trailing
		// space, e.g. "CREATE " + name. Isolated labels such as "create" stay out.
		return strings.HasSuffix(raw, " ")
	}
	return strings.HasPrefix(upper, prefix)
}

func normalizeSQLText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func sqlSnippet(normalized string) string {
	const limit = 160
	if len(normalized) <= limit {
		return normalized
	}
	return normalized[:limit] + "..."
}

func sqlInventoryKey(entry sqlInventoryEntry) string {
	return entry.File + "\x00" + entry.Function + "\x00" + entry.Fingerprint
}

func readSQLInventory() ([]sqlInventoryEntry, error) {
	raw, err := os.ReadFile(sqlInventoryPath)
	if err != nil {
		return nil, err
	}
	var entries []sqlInventoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func writeSQLInventory(entries []sqlInventoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(sqlInventoryPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sqlInventoryPath, append(raw, '\n'), 0o644)
}

func diffSQLInventory(want, got []sqlInventoryEntry) (added, removed []sqlInventoryEntry) {
	wantSet := make(map[string]sqlInventoryEntry, len(want))
	for _, entry := range want {
		wantSet[sqlInventoryKey(entry)] = entry
	}
	gotSet := make(map[string]sqlInventoryEntry, len(got))
	for _, entry := range got {
		gotSet[sqlInventoryKey(entry)] = entry
		if _, ok := wantSet[sqlInventoryKey(entry)]; !ok {
			added = append(added, entry)
		}
	}
	for _, entry := range want {
		if _, ok := gotSet[sqlInventoryKey(entry)]; !ok {
			removed = append(removed, entry)
		}
	}
	sort.Slice(added, func(i, j int) bool { return sqlInventoryKey(added[i]) < sqlInventoryKey(added[j]) })
	sort.Slice(removed, func(i, j int) bool { return sqlInventoryKey(removed[i]) < sqlInventoryKey(removed[j]) })
	return added, removed
}

func TestSQLInventoryDetectsNewLiteral(t *testing.T) {
	if !isSQLText(`SELECT program_name FROM ir_programs WHERE program_name = ?`) {
		t.Fatal("whole SELECT statement was not recognized")
	}
	if !isSQLText(` WHERE program_name = ?`) {
		t.Fatal("SQL clause fragment was not recognized")
	}
	if isSQLText(`get application programs failed: %v`) {
		t.Fatal("ordinary log text was classified as SQL")
	}
}

func TestSQLInventoryDiffReportsAdditions(t *testing.T) {
	oldEntry := sqlInventoryEntry{File: "a.go", Function: "f", Fingerprint: "old"}
	newEntry := sqlInventoryEntry{File: "a.go", Function: "g", Fingerprint: "new"}
	added, removed := diffSQLInventory([]sqlInventoryEntry{oldEntry}, []sqlInventoryEntry{oldEntry, newEntry})
	if len(added) != 1 || added[0] != newEntry {
		t.Fatalf("added = %+v", added)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %+v", removed)
	}
}
