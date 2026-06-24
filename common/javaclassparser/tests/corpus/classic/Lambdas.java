import java.util.Arrays;
import java.util.List;
import java.util.function.BiFunction;
import java.util.function.Function;
import java.util.function.Predicate;
import java.util.stream.Collectors;

public class Lambdas {
    public int apply(int x, BiFunction<Integer, Integer, Integer> f) {
        return f.apply(x, x);
    }

    public int useLambda(int x) {
        BiFunction<Integer, Integer, Integer> add = (a, b) -> a + b;
        Function<Integer, Integer> square = n -> n * n;
        return apply(x, add) + square.apply(x);
    }

    public List<String> stream(List<String> input) {
        return input.stream()
                .filter(s -> s.length() > 2)
                .map(String::toUpperCase)
                .sorted()
                .collect(Collectors.toList());
    }

    public int methodRef(List<Integer> nums) {
        return nums.stream().mapToInt(Integer::intValue).sum();
    }

    public boolean test(Predicate<String> p, String s) {
        return p.test(s);
    }

    public int sum(int... values) {
        return Arrays.stream(values).sum();
    }
}
