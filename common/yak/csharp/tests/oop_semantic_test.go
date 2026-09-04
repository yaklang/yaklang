package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSharp_OOP_MemberFamiliesCompile(t *testing.T) {
	prog := parseCSharpSemantics(t, `
using System;

public delegate int Transformer(int value);

public interface ICounter {
    int Value { get; set; }
    int Increment(int amount);
}

public class BaseCounter {
    protected int seed;
    public BaseCounter(int seed) { this.seed = seed; }
    public virtual int Increment(int amount) => seed + amount;
}

public class Counter : BaseCounter, ICounter {
    public static int Created = 1;
    public int Value { get; set; } = 2;
    public event Action Changed;

    static Counter() { Created += 1; }
    public Counter(int seed) : base(seed) { }

    public override int Increment(int amount) {
        Value += amount;
        return base.Increment(Value);
    }

    public int this[int offset] {
        get => Value + offset;
        set => Value = value;
    }

    public static Counter operator +(Counter counter, int amount) {
        counter.Value += amount;
        return counter;
    }

    ~Counter() { cleanup(Value); }
}

public enum Mode { None, One = 1, Two }

public class Program {
    public static void Main(string[] args) {
        Counter counter = new Counter(3);
        counter[0] = 4;
        Transformer transform = value => counter.Increment(value);
        println(transform(2));
        println(Mode.Two);
    }
}
	`)
	require.NotEmpty(t, prog.Ref("Counter"))
	incrementFlow, err := prog.SyntaxFlowWithError(`.Increment(* as $args)`)
	require.NoError(t, err)
	require.NotEmpty(t, incrementFlow.GetValues("args"), "instance Increment calls must be emitted")
	require.NotEmpty(t, prog.Ref("cleanup"), "finalizer body must be emitted")
	require.NotEmpty(t, prog.Ref("println"))
}

func TestCSharp_OOP_ConstructorAndInstanceFlow(t *testing.T) {
	code := `
public class Box {
    public string Value;
    public Box(string value) { this.Value = value; }
    public string Read() { return Value; }
}

public class Program {
    public static void Main(string[] args) {
        var box = new Box(source());
        sink(box.Read());
    }
}`
	prog := parseCSharpSemantics(t, code)
	result, err := prog.SyntaxFlowWithError(`source() as $source; sink(* #-> as $origin)`)
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("source"))
	require.NotEmpty(t, result.GetValues("origin"), "constructor/member flow must reach sink")
}

func TestCSharp_OOP_ThisConstructorInitializerDoesNotRecurse(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Chained {
    public int Value;
    public Chained() : this(source()) { }
    public Chained(int value) { Value = value; }
}

public class Program {
    public static void Main(string[] args) {
        sink(new Chained());
    }
}
`)
	require.NotEmpty(t, prog.Ref("source"), "constructor initializer arguments must still be emitted")
	require.NotEmpty(t, prog.Ref("sink"))
}
