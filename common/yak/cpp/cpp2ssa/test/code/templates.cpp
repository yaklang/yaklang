template <typename T>
T max_value(T a, T b) {
    return a > b ? a : b;
}

template <class T, int N>
class Buffer {
public:
    Buffer() : size_(N) {}
    void set(int index, T value) { data_[index] = value; }
    T get(int index) const { return data_[index]; }

private:
    T data_[N];
    int size_;
};

template class Buffer<int, 8>;

int use() {
    Buffer<int, 8> buffer;
    buffer.set(0, max_value(1, 2));
    return buffer.get(0);
}
