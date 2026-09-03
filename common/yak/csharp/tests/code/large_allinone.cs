using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;

namespace Large.AllInOne
{
    using Inner = System.Text.StringBuilder;

    [Serializable]
    public abstract class Shape
    {
        public abstract double Area();
    }

    public interface INamed
    {
        string Name { get; }
    }

    public enum Kind { Circle, Rect }

    public delegate double Measure(Shape s);

    public struct Size
    {
        public double W;
        public double H;
        public Size(double w, double h) { W = w; H = h; }
    }

    public class Circle : Shape, INamed
    {
        public double R { get; set; }
        public string Name { get { return "circle"; } }
        public Circle(double r) { R = r; }
        public override double Area() { return 3.14 * R * R; }
        public class Meta { public int Tag; }
    }

    public class Rect : Shape
    {
        public Size Sz;
        public Rect(Size sz) { Sz = sz; }
        public override double Area() { return Sz.W * Sz.H; }
        public double this[int axis]
        {
            get { return axis == 0 ? Sz.W : Sz.H; }
        }
    }

    public static class Catalog
    {
        public static IEnumerable<Shape> Filter(IEnumerable<Shape> xs, double min)
        {
            return from s in xs
                   where s.Area() >= min
                   orderby s.Area() descending
                   select s;
        }

        public static async Task<string> ReportAsync(IEnumerable<Shape> xs)
        {
            var n = xs.Count();
            await Task.Yield();
            return $"shapes={n}";
        }

        public static IEnumerable<int> Range(int n)
        {
            for (int i = 0; i < n; i++)
            {
                yield return i;
            }
        }

        public static int Combine(ref int a, out int b, params int[] rest)
        {
            b = a;
            foreach (var x in rest) { a = a + x; }
            return a;
        }
    }

    public class Program
    {
        public static void Main(string[] args)
        {
            Shape c = new Circle(2);
            Shape r = new Rect(new Size(3, 4));
            var list = new List<Shape> { c, r };
            var filtered = Catalog.Filter(list, 1.0);
            int acc = 0;
            foreach (var s in filtered)
            {
                acc = acc + (int)s.Area();
            }
            switch (acc)
            {
                case 0:
                    acc = 1;
                    break;
                default:
                    acc = acc;
                    break;
            }
            try
            {
                int tmp = 1;
                int o;
                Catalog.Combine(ref tmp, out o, 1, 2, 3);
            }
            catch (Exception ex)
            {
                acc = 0;
            }
            finally
            {
                acc = acc;
            }
            var inner = new Circle.Meta { Tag = 1 };
            Inner sb = new Inner();
            sb.Append(acc);
        }
    }
}
