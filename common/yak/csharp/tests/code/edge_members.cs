using System;

namespace Edge.Members
{
    [Obsolete("demo")]
    public class Widget
    {
        private int _n;
        public event EventHandler Changed;

        public int Count
        {
            get { return _n; }
            set { _n = value; }
        }

        public int this[int i]
        {
            get { return _n + i; }
            set { _n = value - i; }
        }

        public Widget()
        {
            _n = 0;
        }

        public void Mutate(ref int a, out int b, params int[] rest)
        {
            a = a + 1;
            b = a;
            if (rest != null && rest.Length > 0)
            {
                _n = rest[0];
            }
            if (Changed != null)
            {
                Changed(this, EventArgs.Empty);
            }
        }

        public static int Sum(in int x, int y)
        {
            return x + y;
        }
    }
}
