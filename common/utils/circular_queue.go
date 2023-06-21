package utils

type CircularQueue struct {
	data  []interface{}
	front int
	rear  int
	size  int
}

func NewCircularQueue(capacity int) *CircularQueue {
	return &CircularQueue{
		data: make([]interface{}, capacity),
		size: 0,
	}
}

func (q *CircularQueue) Push(x interface{}) {
	if q.size < cap(q.data) { // 队列未满，直接插入到队尾
		q.data[q.rear] = x
		q.rear = (q.rear + 1) % cap(q.data)
		q.size++
	} else { // 队列已满，覆盖队首元素
		q.data[q.front] = x
		q.front = (q.front + 1) % cap(q.data)
		q.rear = (q.rear + 1) % cap(q.data)
	}
}

func (q *CircularQueue) GetElements() []interface{} {
	if q == nil {
		return nil
	}
	elements := make([]interface{}, 0, q.size)

	for i := 0; i < q.size; i++ {
		index := (q.front + i) % cap(q.data)
		elements = append(elements, q.data[index])
	}

	return elements
}
