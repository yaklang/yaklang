// Operator overload alternatives from CSharpParser.g4 (unary, binary, conversion).
using System;

public class Ops
{
    public int V;
    public Ops(int v) { V = v; }

    public static Ops operator +(Ops a) { return a; }
    public static Ops operator -(Ops a) { return new Ops(-a.V); }
    public static Ops operator !(Ops a) { return new Ops(a.V == 0 ? 1 : 0); }
    public static Ops operator ~(Ops a) { return new Ops(~a.V); }
    public static Ops operator ++(Ops a) { return new Ops(a.V + 1); }
    public static Ops operator --(Ops a) { return new Ops(a.V - 1); }
    public static bool operator true(Ops a) { return a.V != 0; }
    public static bool operator false(Ops a) { return a.V == 0; }

    public static Ops operator +(Ops a, Ops b) { return new Ops(a.V + b.V); }
    public static Ops operator -(Ops a, Ops b) { return new Ops(a.V - b.V); }
    public static Ops operator *(Ops a, Ops b) { return new Ops(a.V * b.V); }
    public static Ops operator /(Ops a, Ops b) { return new Ops(a.V / b.V); }
    public static Ops operator %(Ops a, Ops b) { return new Ops(a.V % b.V); }
    public static Ops operator &(Ops a, Ops b) { return new Ops(a.V & b.V); }
    public static Ops operator |(Ops a, Ops b) { return new Ops(a.V | b.V); }
    public static Ops operator ^(Ops a, Ops b) { return new Ops(a.V ^ b.V); }
    public static Ops operator <<(Ops a, int b) { return new Ops(a.V << b); }
    public static Ops operator >>(Ops a, int b) { return new Ops(a.V >> b); }
    public static bool operator ==(Ops a, Ops b) { return a.V == b.V; }
    public static bool operator !=(Ops a, Ops b) { return a.V != b.V; }
    public static bool operator >(Ops a, Ops b) { return a.V > b.V; }
    public static bool operator <(Ops a, Ops b) { return a.V < b.V; }
    public static bool operator >=(Ops a, Ops b) { return a.V >= b.V; }
    public static bool operator <=(Ops a, Ops b) { return a.V <= b.V; }

    public static implicit operator int(Ops a) { return a.V; }
    public static explicit operator Ops(int a) { return new Ops(a); }

    public override bool Equals(object o) { return o is Ops other && other.V == V; }
    public override int GetHashCode() { return V; }
}
