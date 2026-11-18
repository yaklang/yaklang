namespace demo {

class Shape {
public:
    Shape() : id_(0) {}
    virtual ~Shape() {}
    virtual double area() const = 0;
    int id() const { return id_; }

protected:
    int id_;
};

class Circle : public Shape {
public:
    explicit Circle(double r) : radius_(r) {}
    ~Circle() override {}
    double area() const override { return 3.14 * radius_ * radius_; }

private:
    double radius_;
};

double measure(const Shape& shape) {
    return shape.area();
}

}
