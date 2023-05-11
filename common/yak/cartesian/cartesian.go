package cartesian

import (
	"context"
)

func Product[T any](origin [][]T) ([][]T, error) {
	var result [][]T
	var err = ProductExContext[T](context.Background(), origin, func(raw []T) error {
		result = append(result, raw)
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func ProductEx[T any](origin [][]T, handler func(result []T) error) error {
	return ProductExContext[T](context.Background(), origin, handler)
}

func ProductExContext[T any](ctx context.Context, origin [][]T, handler func(raw []T) error) error {
	ch := make(chan []T)
	ctx1, cancel := context.WithCancel(ctx)
	defer cancel()
	go cartesianProductRaw[T](ctx1, origin, ch)
	for raw := range ch {
		if err := handler(raw); err != nil {
			return err
		}
	}
	return nil
}

func cartesianProductRaw[T any](ctx context.Context, sets [][]T, ch chan<- []T) {
	n := len(sets)
	positions := make([]int, n)
	done := n == 0

	for !done {
		product := make([]T, n)
		for i, pos := range positions {
			product[i] = sets[i][pos]
		}

		select {
		case <-ctx.Done():
			close(ch)
			return
		case ch <- product:
		}

		i := n - 1
		for i >= 0 {
			if positions[i] < len(sets[i])-1 {
				positions[i]++
				break
			} else {
				positions[i] = 0
				i--
			}
		}
		done = i < 0
	}

	close(ch)
}
