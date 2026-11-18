#include <string>
#include <vector>

namespace outer {
namespace inner {
int value() { return 1; }
}
}

using namespace outer::inner;
using std::string;

namespace alias = outer;

typedef unsigned long ulong;

int run() {
    string name = "cpp";
    std::vector<int> numbers;
    numbers.push_back(value());
    ulong total = numbers.size();
    return total;
}
