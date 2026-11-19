#include <vector>
namespace outer { namespace inner {

template <typename T, int N = 4, typename... Rest>
class Box : public Base<T> {
public:
    template <typename U>
    U convert(const U& value) const { return value; }
    typename T::value_type pick(typename T::value_type v) { return v; }
    template <template <typename> class Tmpl>
    void apply(Tmpl<T> t) {}
};

template <typename T, int N, typename... Rest>
class Box<T*, N, Rest...> : public Base<T> {};

template <>
class Box<int, 4> {};

template <typename T>
using Vec = std::vector<T>;

template <typename T>
struct Traits { using type = typename T::value_type; };

template <typename T>
void call(typename Traits<T>::type v) {}

template <typename T>
void callee(T t) { t.template rebind<int>(); }

extern template class Box<double>;

}}
