using System;

public class ModOuter
{
    protected class CProt { }
    internal class CInt { }
    private class CPriv { }

    protected const int KProt = 1;
    internal const int KInt = 2;
    private const int KPriv = 3;
    public const int KPub = 4;
    new const int KNew = 5;

    protected struct SProt { public int X; }
    internal struct SInt { public int X; }
    private struct SPriv { public int X; }
    public readonly struct SRo { public readonly int X; }
    new struct SNew { public int X; }

    protected enum EProt { A }
    internal enum EIntE { A }
    private enum EPriv { A }
    new enum ENew { A }

    protected delegate void DProt();
    internal delegate void DInt();
    private delegate void DPriv();
    new delegate void DNew();
    public unsafe delegate void DUnsafe();

    protected interface IProt { }
    internal interface IInt { }
    private interface IPriv { }
    new interface INew { }
    public unsafe interface IUnsafe { void M(); }

    protected event EventHandler EvProt;
    internal event EventHandler EvInt;
    private event EventHandler EvPriv;
    static event EventHandler EvStat;
    public virtual event EventHandler EvVirt;
    public abstract event EventHandler EvAbs { add { } remove { } }
    public event EventHandler EvExt { add { } remove { } }
    public sealed override event EventHandler EvSealed { add { } remove { } }
    public new event EventHandler EvNew;
    public extern event EventHandler EvExtern;
    public unsafe event EventHandler EvUnsafe;

    protected int FProt;
    internal int FInt;
    private int FPriv;
    public static int FStat;
    public readonly int FRo;
    public volatile int FVol;
    public unsafe int* FPtr;
    public new int FNew;

    protected int PProt { get; set; }
    internal int PInt { get; set; }
    private int PPriv { get; set; }
    public static int PStat { get; set; }
    public virtual int PVirt { get; set; }
    public abstract int PAbs { get; set; }
    public override int POver { get { return 1; } }
    public sealed override int PSeal { get { return 1; } }
    public new int PNew { get; set; }
    public extern int PExt { get; set; }
    public unsafe int PUnsafe { get; set; }

    protected int this[int i] { get { return i; } set { } }
    internal int this[long i] { get { return (int)i; } }
    private int this[string s] { get { return 0; } }
    public virtual int this[byte b] { get { return b; } }
    public abstract int this[short s] { get; }
    public override int this[ushort s] { get { return s; } }
    public sealed override int this[uint u] { get { return (int)u; } }
    public new int this[ulong u] { get { return (int)u; } }
    public extern int this[char c] { get; set; }
    public readonly int this[float f] { get { return 0; } }
    public unsafe int this[double d] { get { return 0; } }

    protected ModOuter() { }
    internal ModOuter(int a) { }
    private ModOuter(string s) { }
    public extern ModOuter(long x);
    public unsafe ModOuter(int* p) { }

    public int Get { get { return 1; } private set { } }
    public int Get2 { get { return 1; } protected set { } }
    public int Get3 { get { return 1; } internal set { } }
    public int Get4 { get { return 1; } protected internal set { } }
    public int Get5 { get { return 1; } internal protected set { } }
    public int Get6 { get { return 1; } protected private set { } }
    public int Get7 { get { return 1; } private protected set { } }
    public readonly int Get8 { get { return 1; } }

    public unsafe struct NestedFixed
    {
        public new fixed int A[2];
        public internal fixed int B[2];
        private fixed int C[2];
        public unsafe fixed int D[2];
        public fixed int E[2];
    }
}

public abstract class ModAbstract : ModOuter
{
    public abstract event EventHandler EvAbs2;
    public abstract int PAbs2 { get; set; }
    public abstract int this[decimal d] { get; }
}

public unsafe class UnsafeCtor
{
    static UnsafeCtor() { }
    static extern UnsafeCtor();
    extern static UnsafeCtor();
    static extern unsafe UnsafeCtor();
    extern unsafe static UnsafeCtor();
    unsafe static extern UnsafeCtor();
    unsafe extern static UnsafeCtor();
}

public unsafe class UnsafeOps
{
    public static extern int operator +(UnsafeOps a, UnsafeOps b);
    public static unsafe int operator -(UnsafeOps a, UnsafeOps b) { return 0; }
}

public interface IMembers
{
    const int C = 1;
    static int F;
    static IMembers() { }
    static int operator +(IMembers a, int b) { return b; }
    class Nested { }
}
