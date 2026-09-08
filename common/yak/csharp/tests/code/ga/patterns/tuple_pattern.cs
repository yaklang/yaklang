namespace Ga.Patterns {
    public class GaPatternsTuple {
        public static string Who(string a, string b) => (a, b) switch {
            ("x", "y") => "xy",
            var t when t.Item1 == t.Item2 => "eq",
            _ => "no",
        };
    }
}
