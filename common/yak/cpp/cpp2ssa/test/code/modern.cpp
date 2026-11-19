#include <vector>

enum class Color { Red, Green, Blue };

struct Point {
    int x;
    int y;
};

int modern() {
    auto total = 0;
    std::vector<int> values{1, 2, 3};
    for (auto value : values) {
        total += value;
    }
    Point point{1, 2};
    total += point.x;
    Color color = Color::Red;
    if (color == Color::Red && nullptr != &point) {
        total += static_cast<int>(2.5);
    }
    try {
        throw 1;
    } catch (int code) {
        total += code;
    }
    return total;
}
