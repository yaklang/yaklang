using System;

namespace Demo
{
    public class Point
    {
        public int X;
        public int Y;

        public Point(int x, int y)
        {
            X = x;
            Y = y;
        }

        public int Sum()
        {
            return X + Y;
        }
    }

    public interface IPrinter
    {
        void Print(string s);
    }

    public struct Box
    {
        public int W;
        public int H;
    }

    public class Program
    {
        public static void Main(string[] args)
        {
            int a = 1;
            string s = "hello";
            Point p = new Point(1, 2);
            Console.WriteLine(p.Sum());
        }
    }
}
