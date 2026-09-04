using System;
using System.Collections.Generic;

public class RemainingAlts : BaseRemain
{
    public int Auto;
    public DRemain Del;
    public ERemain En;

    public RemainingAlts() : base() { }

    public void M(DRemain d)
    {
        int x = 1, y = 2;
        object o = this;
        x++;
        x--;
        --x;
        d = new DRemain(Local);
        typeof(Dictionary<,>);
        typeof(global::System.Collections.Generic.List<>);
        nameof(this);
        nameof(base);
        nameof(x);
        nameof(global::System.Int32);

        void Local() { }
        void LocalNc() => o?.ToString();
        ref int LocalRef() { return ref x; }
        unsafe int LocalUnsafe() { return 1; }

        System.Func<object> f1 = () => o?.ToString();
        System.Func<int> f2 = () => ref x;
        System.Action<int> f3 = (out int z) => { z = 1; };
        System.Action<int> f4 = (ref int z) => { z++; };
        System.Action<int> f5 = (in int z) => { };

        var anon = new { x, base.Id, o?.ToString, int.MaxValue, global::System.Math.PI, };
        var obj = new RemainingAlts { Auto = 1, Nested = { Auto = 2 } };
        var obj2 = new RemainingAlts { Auto = 1, };

        if (o is var v) { }
        if (o is _) { }
        if (o is var (a, b)) { }
        if (o is (var p, var q)) { }

        switch (x, y)
        {
            case (0, 0):
                break;
            default:
                break;
        }

        foreach (var item in new[] { 1 })
        {
            base[0] = item;
        }

        int[] xs = { 1, 2 };
        var n = xs?[0];
        var s = o?.ToString()?[0];
        void ArrowNc() => o?.ToString();
    }

    public void ArrowNc2() => ((object)this)?.ToString();
    public object Nested;
}

public class BaseRemain
{
    public int Id { get { return 1; } }
    public int this[int i] { get { return i; } set { } }
}

public delegate void DRemain();
public enum ERemain : System.Int32 { A }

public class Gen<T, U>
    where T : class
    where U : T
{
    public void M(T t, U u, dynamic d) { }
}

public unsafe class PtrRemain
{
    public void M()
    {
        int v = 1;
        int* p = &v;
        sizeof(int*);
    }
}

public interface IExplicit
{
    int this[int i] { get; }
}

public class ExplicitIdx : IExplicit
{
    int IExplicit.this[int i] { get { return i; } }
}

public struct SConst
{
    public const int C = 1;
    static SConst() { }
    public int RoAcc { readonly get { return C; } }
}

public class MoreAlts
{
    internal ref int InnerRef(ref int x) { return ref x; }
    public ref int RefGet
    {
        get => ref field;
    }
    public ref int RefGetEmpty
    {
        get;
    }
    int field;

    public void Patterns(object o, int[] xs)
    {
        int __arglist = 1;
        switch (o)
        {
            case var x when true:
                break;
            case _ when true:
                break;
            case var (a, b) when true:
                break;
            case var ((c, d), e) when true:
                break;
        }
        object idx = o?.Item[0];
        var ((p, q), r) = ((1, 2), 3);
        dynamic[] dyns = null;
        object n = o?.ToString()?[0];
        object m = xs?[0];
        object k = nameof(MoreAlts.Patterns);
        object k2 = nameof(global::System.Math.PI);
        object k3 = nameof(this.field);
    }

}

public class Box<T> { }

public class Constraints<T, U, V>
    where T : notnull
    where U : unmanaged
    where V : T?
{
    public Box<T?> BoxT;
}

public unsafe class PtrArr
{
    public int*[] Ptrs;
}
