#include <iostream>
#include <map>
#include <memory>
#include <string>
#include <vector>

class Resource {
public:
    Resource() : value_(0) {}
    ~Resource() {}
    void use() { value_ = 1; }
    int value() const { return value_; }

private:
    int value_;
};

void consume() {
    std::unique_ptr<Resource> resource(new Resource());
    std::shared_ptr<Resource> shared = std::make_shared<Resource>();
    resource->use();
    shared->use();
    std::vector<std::string> names;
    names.push_back("a");
    std::map<std::string, int> counters;
    counters["a"] = 1;
    std::cout << names.size() << counters.size() << std::endl;
    auto lambda = [](int x) { return x + 1; };
    lambda(1);
}
