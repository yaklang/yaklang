using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;

namespace Edge.AsyncLinq
{
    public class QueryBox
    {
        public async Task<int> AddAsync(int a, int b)
        {
            await Task.Yield();
            return a + b;
        }

        public IEnumerable<int> Odds(IEnumerable<int> xs)
        {
            foreach (var x in xs)
            {
                if (x % 2 == 1)
                {
                    yield return x;
                }
            }
            yield break;
        }

        public IEnumerable<int> Filter(IEnumerable<int> xs)
        {
            return from x in xs
                   where x > 0
                   orderby x
                   select x;
        }

        public string Format(string name, int n)
        {
            return $"hello {name} count={n}";
        }

        public string Verbatim(string path)
        {
            return $@"C:\tmp\{path}";
        }
    }
}
