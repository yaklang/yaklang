package utils

type Stack struct {
	elements []interface{}
}

func NewStack() *Stack {
	return &Stack{}

}
func (s *Stack) Push(element interface{}) {
	s.elements = append(s.elements, element)
}
func (s *Stack) Peek() interface{} {
	if len(s.elements) == 0 {
		return nil
	}
	return s.elements[len(s.elements)-1]
}

func (s *Stack) Pop() interface{} {
	if len(s.elements) == 0 {
		return nil
	}
	element := s.elements[len(s.elements)-1]
	s.elements = s.elements[:len(s.elements)-1]
	return element
}

func (s *Stack) IsEmpty() bool {
	return len(s.elements) == 0
}
func (s *Stack) Size() int {
	return len(s.elements)
}
