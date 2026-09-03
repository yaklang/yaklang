using System;
using System.Collections.Generic;

namespace Edge.Types
{
    public enum Color : byte
    {
        Red = 1,
        Green,
        Blue
    }

    public delegate int Mapper<T>(T value);

    public interface IRepo<T> where T : class, new()
    {
        T Get(int id);
    }

    public struct Pair<T, U>
        where T : struct
        where U : class
    {
        public T Left;
        public U Right;
    }

    public class NestedHost<T> where T : IComparable<T>
    {
        public class Inner
        {
            public T Value;
        }

        public struct InnerBox
        {
            public int N;
        }

        public enum InnerKind
        {
            A,
            B
        }
    }

    public class Uses
    {
        public static void Run()
        {
            Mapper<int> m = x => x + 1;
            var p = new Pair<int, string> { Left = 1, Right = "a" };
            var inner = new NestedHost<int>.Inner();
            Color c = Color.Red;
        }
    }
}
