#include <tuple>
namespace app::detail {
inline namespace v1 { struct S {}; }
}
template <typename... Args>
auto total(Args... args) {
    return (args + ...);
}
template <typename... Args>
auto total_init(Args... args) {
    return (0 + ... + args);
}
template <typename T, typename... Args>
bool any_of(T first, Args... rest) {
    return (first || ... || rest);
}
int main() {
    auto [a, b] = std::pair<int, int>{1, 2};
    constexpr bool flag = true;
    if constexpr (flag) { a = 1; } else { b = 2; }
    auto sum = total(1, 2, 3);
    auto total2 = total_init(1, 2, 3);
    return a + b + sum + total2 + (any_of(true, false) ? 1 : 0);
}
