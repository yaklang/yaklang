#line 1
#error parse-coverage-directive-text
#warning parse-coverage-directive-text
#region coverage
#pragma warning disable
#nullable enable
#undef MISSING
extern alias CoreLib;

using System;
using System.Collections.Generic;
using System.Linq;
using static System.Math;
using IO = System.IO;
using Cons = System.Console;

[assembly: System.Obsolete]
[module: System.CLSCompliant(false)]

namespace Coverage.Alts
{
    using System.Text;

    public delegate void D1();
    public delegate ref int D2();
    public delegate void D3<in T, out U>(T x) where T : class where U : class;

    public enum EByte : byte { A = 1, B }
    public enum ESbyte : sbyte { A }
    public enum EShort : short { A }
    public enum EUShort : ushort { A }
    public enum EInt : int { A }
    public enum EUInt : uint { A }
    public enum ELong : long { A }
    public enum EULong : ulong { A }
    public enum EChar : char { A = 'A' }

    public interface IInOut<in T, out U>
    {
        U Map(T x);
        int P { get; set; }
        event EventHandler Evt;
        int this[int i] { get; }
    }

    public interface IDefault
    {
        void M();
        void N() { }
        static int S() { return 1; }
    }

    public readonly struct RS
    {
        public readonly int X;
        public RS(int x) { X = x; }
        public readonly int Get() { return X; }
    }

    public ref struct RefS
    {
        public int X;
        public void Dispose() { }
    }

    public struct Pair
    {
        public int X;
        public int Y;
        public Pair(int x, int y) { X = x; Y = y; }
        public void Deconstruct(out int x, out int y) { x = X; y = Y; }
    }

    public unsafe struct FixedBuf
    {
        public fixed int Data[4];
        public new int Hidden;
        public internal int Mix1;
        private int Mix2;
    }

    public abstract class Base
    {
        protected Base() { }
        protected Base(int x) : this() { }
        public abstract int Area();
        public virtual int Id() { return 0; }
        ~Base() { }
    }

    public sealed class SealedLeaf : Base, IDefault
    {
        public SealedLeaf() : base(1) { }
        static SealedLeaf() { }
        public override int Area() { return 1; }
        public sealed override int Id() { return 2; }
        public void M() { }
        public int this[int i] { get { return i; } set { } }
        public int this[string s] { get => s.Length; }
    }

    public partial class Host
    {
        public const int C = 1;
        public static readonly int Sr = 2;
        public volatile int Vol;
        public event EventHandler Changed;
        public event EventHandler Custom
        {
            add { Changed += value; }
            remove { Changed -= value; }
        }

        public int Auto { get; set; } = 3;
        public int Expr => 4;
        public ref int RefProp
        {
            get { return ref Auto; }
        }
        public ref int RefExpr => ref Auto;

        public int this[int i, int j]
        {
            get { return i + j; }
            set { Auto = value; }
        }

        public ref int this[long i]
        {
            get { return ref Auto; }
        }

        public Host() { }
        public Host(int x) : this() { Auto = x; }

        public void Ext(this int x) { }
        public void M(int a = 1, params int[] rest) { }
        public void N(in int a, ref int b, out int c) { c = a + b; }
        public void Arglist(__arglist) { }

        public int Body() { return 1; }
        public int Arrow() => 2;
        public void Empty();
        public ref int RefM() { return ref Auto; }
        public ref int RefArrow() => ref Auto;

        public static async System.Threading.Tasks.Task<int> AsyncM()
        {
            await System.Threading.Tasks.Task.Yield();
            return 1;
        }

        public IEnumerable<int> Y()
        {
            yield return 1;
            yield break;
        }

        public object Query(int[] xs, int[] ys)
        {
            var q1 = from x in xs where x > 0 orderby x ascending, x descending select x;
            var q2 = from x in xs
                     join y in ys on x equals y
                     let z = x + y
                     group z by z % 2 into g
                     select g;
            var q3 = from x in xs
                     join y in ys on x equals y into gj
                     from y in gj
                     select x + y;
            return q2;
        }

        public void Stmts(int n, int[] xs)
        {
            ;
            L:
            if (n > 0) n++;
            else n--;
            do { n--; } while (n > 10);
            for (;;) { break; }
            for (n = 0; n < 1; n++) continue;
            foreach (var x in xs) { n += x; }
            foreach (ref var x in xs) { x++; }
            foreach (var (a, b) in new[] { (1, 2) }) { n += a + b; }
            switch (n)
            {
                case 0 when n == 0:
                    goto case 1;
                case 1:
                    goto default;
                case Pair (int px, int py):
                    n += px + py;
                    break;
                case { Auto: 1 }:
                    break;
                case int k:
                    n += k;
                    break;
                case var z:
                    n += z == null ? 0 : 1;
                    break;
                default:
                    break;
            }
            try { throw new System.Exception(); }
            catch (System.Exception ex) when (ex != null) { n = 0; }
            catch { n = 1; }
            finally { n = n; }
            try { n = 1; } finally { n = 2; }
            checked { n = n + 1; }
            unchecked { n = n + 1; }
            lock (this) { n = n; }
            using (var r = (IDisposable)null) { n = n; }
            using var u = (IDisposable)null;
            await using var au = (IAsyncDisposable)null;
            int local = 1;
            const int lc = 2;
            void Local() { local++; }
            static int SLocal(int x) => x + 1;
            async void ALocal() { await System.Threading.Tasks.Task.Yield(); }
            ref int RLocal() => ref Auto;
            n = SLocal(local + lc);
            goto L;
        }

        public object Exprs(int a, int b, Ops o, int[] xs, Host h)
        {
            int x = 1, y = 2;
            x += 1; x -= 1; x *= 1; x /= 1; x %= 1; x &= 1; x |= 1; x ^= 1; x <<= 1; x >>= 1;
            object nn = null;
            nn ??= x;
            ref int rx = ref x;
            x = ref y;
            var t = (x, y);
            var (l, r) = t;
            (x, y) = (y, x);
            var anon = new { x, y, h.Auto, N = 1 };
            var arr1 = new int[] { 1, 2, };
            var arr2 = new int[2] { 1, 2 };
            var arr3 = new[] { 1, 2 };
            var arr4 = new int[1, 2];
            var obj = new Host { Auto = 1, [0, 1] = 2 };
            var list = new List<int> { 1, 2, };
            D1 d = new D1(LocalD);
            var tp = typeof(int);
            var tv = typeof(void);
            var tu = typeof(List<>);
            var sz = sizeof(int);
            var dv = default(int);
            int dl = default;
            var ne = nameof(Auto);
            var nt = nameof(int);
            var nthis = nameof(this);
            int ckd = checked(a + b);
            int uc = unchecked(a + b);
            int pinc = ++x;
            int pdec = --x;
            x++; x--;
            bool bt = true, bf = false;
            object nu = null;
            char ch = 'a';
            string s = "hi";
            string vs = @"c:\x";
            string ir = $"v={x,-2:d}";
            string iv = $@"v={x}";
            string iv2 = @$"v={x}";
            int rng1 = xs[^1];
            int[] rng2 = xs[1..3];
            int[] rng3 = xs[..];
            int[] rng4 = xs[..3];
            int[] rng5 = xs[1..];
            object sw = a switch { 0 => 1, 1 => 2, _ => 3 };
            bool isd = a is int;
            bool isp = h is { Auto: 1 };
            Host ash = h as Host;
            int sh = a >> 1;
            int and = a & b | b ^ a;
            bool cand = a > 0 && b > 0 || a == b;
            int cond = a > 0 ? a : b;
            ref int rcond = ref (a > 0 ? ref x : ref y);
            System.Action act = () => { x++; };
            System.Func<int, int> f1 = (int z) => z;
            System.Func<int, int> f2 = z => z;
            System.Func<int> f3 = async () => { await System.Threading.Tasks.Task.Yield(); return 1; };
            System.Func<int> f4 = delegate { return 1; };
            System.Func<int, int> f5 = delegate (int z) { return z; };
            var lamRef = (ref int z) => z;
            int q = a * b / 2 % 3 + 4 - 1;
            bool eq = a == b && a != b && a <= b && a >= b && a < b && a > b;
            object th = a > 0 ? a : throw new System.Exception();
            h?.Auto.ToString();
            h?[0, 1].ToString();
            xs?[0].ToString();
            string nf = s!;
            int mk = __refvalue(__makeref(x), int);
            var rt = __reftype(__makeref(x));
            LocalD();
            return tu;

            void LocalD() { }
        }

        public unsafe void UnsafeBits(int[] xs)
        {
            int v = 1;
            int* p = &v;
            *p = 2;
            void* vp = p;
            int** pp = &p;
            fixed (int* fp = xs)
            {
                int n = fp[0];
            }
            Point2* pt;
        }
    }

    public unsafe struct Point2
    {
        public int X;
        public int Y;
        public void Use(Point2* p)
        {
            int x = p->X;
        }
    }

    public unsafe class FinalA
    {
        ~FinalA() { }
    }

    public unsafe class FinalB
    {
        extern ~FinalB();
    }

    public unsafe class FinalC
    {
        extern unsafe ~FinalC();
    }

    public class NestedNew
    {
        public class Inner { }
        public new class Hidden { }
        public struct S { }
        public enum K { A }
        public delegate void D();
        public interface I { }
    }

    public static class StaticHost
    {
        public static int X;
        static StaticHost() { }
    }

    public class GlobalAlias
    {
        public object M()
        {
            return global::System.Int32.MaxValue;
        }
    }
}
#endregion
