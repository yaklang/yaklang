// Member-level fixtures: methods, operators, statics and templates.

struct Vec2 {
    int x;
    int y;
};

Vec2 operator+(const Vec2& left, const Vec2& right) {
    Vec2 result;
    result.x = left.x + right.x;
    result.y = left.y + right.y;
    return result;
}

class Counter {
public:
    Counter() : value_(0), step_(1) {}
    ~Counter() { value_ = 0; }

    int bump() { value_ += step_; return value_; }
    int value() const { return value_; }
    static int instances() { return instances_; }

private:
    int value_;
    int step_;
    static int instances_;
};

int Counter::instances_ = 0;

template <typename T>
class Box {
public:
    Box() : value_() {}
    T get() const { return value_; }
    void set(T value) { value_ = value; }

private:
    T value_;
};

int useMembers() {
    Counter counter;
    counter.bump();
    counter.bump();
    Box<int> box;
    box.set(3);
    return counter.value() + box.get() + Counter::instances();
}