#include <vector>
#include <utility>
int add(int a, int b) noexcept { return a + b; }
[[nodiscard]] int guard() { return 1; }
int main() {
    int base = 3;
    auto byValue = [=](int x) { return x + base; };
    auto byRef = [&](int& x) mutable { x += base; };
    auto generic = [](auto a, auto b) { return a + b; };
    auto trailing = [](int* p) -> decltype(*p) { return *p; };
    std::vector<int> v{1, 2, 3};
    int sum = 0;
    for (auto& item : v) { sum += item; }
    for (const auto& [key, value] : std::vector<std::pair<int,int>>{{1,2}}) { sum += key + value; }
    auto d = decltype(sum){};
    (void)d;
    return generic(byValue(1), add(1, 2)) + byRef, sum;
}
