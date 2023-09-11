package workpool

type WorkPool[T any] struct {
	workerCount int
	jobQueue    chan T
	workerF     func(chan T)
}

func New[T any](workerCount int, workerF func(chan T)) *WorkPool[T] {
	return &WorkPool[T]{
		workerCount: workerCount,
		jobQueue:    make(chan T, 1000),
		workerF:     workerF,
	}
}

func (wp *WorkPool[T]) Start() {
	for i := 0; i < wp.workerCount; i++ {
		go wp.workerF(wp.jobQueue)
	}
}

func (wp *WorkPool[T]) Stop() {
	close(wp.jobQueue)
}

func (wp *WorkPool[T]) AddJob(job T) {
	wp.jobQueue <- job
}
