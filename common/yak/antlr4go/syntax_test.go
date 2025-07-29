package antlr4go

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func SyntaxBase(code string, info bool) (*gol.SourceFileContext, error) {
	lexer := gol.NewGoLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()

	if info {
		tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		tokenSource := tokenStream.GetTokenSource()
		count := 0
		for {
			t := tokenSource.NextToken()
			count++
			_ = t
			if t.GetTokenType() == antlr.TokenEOF {
				break
			}
			fmt.Printf("%v\n", t)
		}
	}

	errListener := antlr4util.NewErrorListener()
	lexer = gol.NewGoLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := gol.NewGoParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	ast := parser.SourceFile().(*gol.SourceFileContext)
	if len(errListener.GetErrors()) != 0 {
		err := utils.Errorf("[-]parse AST FrontEnd error : %v", errListener.GetErrorString())
		return ast, err
	}
	tree := ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser())
	fmt.Printf("[+]tree: %v\n", tree)
	return ast, nil
}

type Container []struct {
	Items int
}

func TestExample(t *testing.T) {
	code := `package main

    import "fmt"

    func main() { 
		fmt.Println("Hello, world!") 
	}`
	_, err := SyntaxBase(code, true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExamplesAll(t *testing.T) {
	exdir := "./test"
	err := filepath.Walk(exdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fmt.Println("[+]start test:", path)
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Println("Error reading the file:", err)
				return nil
			}
			_, err = SyntaxBase(string(data), false)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println("-------------------------------------------")
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error walking the path:", err)
	}
}

func TestExample_Dot(t *testing.T) {
	code := `package main

func (m Migrator) GetTables() (tableList []string, err error) {
	err = m.DB.Raw("SELECT TABLE_NAME FROM information_schema.tables where TABLE_SCHEMA=?", m.CurrentDatabase()).
		Scan(&tableList).Error
	return
}
`
	_, err := SyntaxBase(code, true)
	if err != nil {
		t.Fatal(err)
	}
}
