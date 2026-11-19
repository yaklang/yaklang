#ifndef CPP_TEST_CIRCLE_HPP
#define CPP_TEST_CIRCLE_HPP

class Circle {
public:
    Circle(int radius) : radius_(radius) {}
    int area() const { return radius_ * radius_; }
    int radius() const { return radius_; }

private:
    int radius_;
};

Circle makeCircle(int radius);

#endif