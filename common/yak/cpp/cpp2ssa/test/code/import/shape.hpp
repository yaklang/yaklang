#ifndef CPP_TEST_SHAPE_HPP
#define CPP_TEST_SHAPE_HPP

int helper(int value);

class Adder {
public:
    Adder() : base_(1) {}
    int add(int value) const { return base_ + value; }

private:
    int base_;
};

#define CPP_TEST_SCALE 3

#endif