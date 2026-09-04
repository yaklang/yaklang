using System;
namespace Ga.Lambdas {
    public class GaLambdasSimple {
        public static int Run() {
            Func<int, int> f = x => x + 1;
            return f(2);
        }
    }
}
