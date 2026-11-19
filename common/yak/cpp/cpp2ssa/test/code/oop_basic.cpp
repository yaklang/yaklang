// Class fixtures: fields, inheritance and virtual methods.

class Shape {
public:
    Shape() : id_(0) {}
    virtual ~Shape() {}
    virtual double area() const { return 0.0; }
    int id() const { return id_; }

protected:
    int id_;
};

class Rectangle : public Shape {
public:
    Rectangle(double width, double height) : width_(width), height_(height) {}
    double area() const { return width_ * height_; }
    double width() const { return width_; }

private:
    double width_;
    double height_;
};

class Square : public Rectangle {
public:
    explicit Square(double side) : Rectangle(side, side) {}
    double area() const { return width() * width(); }
};

double totalArea(Shape* first, Shape* second) {
    return first->area() + second->area();
}

int useShapes() {
    Rectangle rectangle(2.0, 3.0);
    Square square(4.0);
    return (int)(rectangle.area() + square.area());
}