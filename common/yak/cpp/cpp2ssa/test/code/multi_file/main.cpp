#include "math.hpp"
#include "circle.hpp"

int compute(int value) {
    Circle circle = makeCircle(value);
    return square(value) + cube(value) + circle.area();
}

int main() {
    return compute(3);
}