package tests

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"testing"
)

//go:embed basic1.class
var basic1 []byte

func TestBasicCase(t *testing.T) {
	_ = `
public class SimpleCalculator {
    private int value;

    public SimpleCalculator() {
        this.value = 0;
    }

    public int add(int num) {
        this.value += num;
        return this.value;
    }

    public int subtract(int num) {
        this.value -= num;
        return this.value;
    }

    public int getValue() {
        return this.value;
    }

    public static void main(String[] args) {
        SimpleCalculator calc = new SimpleCalculator();
        System.out.println("Initial value: " + calc.getValue());
        System.out.println("After adding 5: " + calc.add(5));
        System.out.println("After subtracting 2: " + calc.subtract(2));
    }
}
`
	s, err := javaclassparser.Decompile(basic1)
	if err != nil {
		t.Fatal(err)
	}
	println(s)
}
