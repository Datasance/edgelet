package utils //nolint:revive // legacy package name

// Either represents a value that can be either Left (error) or Right (success)
type Either[T any] interface {
	IsLeft() bool
	IsRight() bool
	Left() error
	Right() T
}

// Left represents an error value
type Left[T any] struct {
	err error
}

// NewLeft creates a new Left value
func NewLeft[T any](err error) *Left[T] {
	return &Left[T]{err: err}
}

// IsLeft returns true
func (l *Left[T]) IsLeft() bool {
	return true
}

// IsRight returns false
func (l *Left[T]) IsRight() bool {
	return false
}

// Left returns the error
func (l *Left[T]) Left() error {
	return l.err
}

// Right panics (Left has no right value)
func (l *Left[T]) Right() T {
	panic("Left has no right value")
}

// Right represents a success value
type Right[T any] struct {
	value T
}

// NewRight creates a new Right value
func NewRight[T any](value T) *Right[T] {
	return &Right[T]{value: value}
}

// IsLeft returns false
func (r *Right[T]) IsLeft() bool {
	return false
}

// IsRight returns true
func (r *Right[T]) IsRight() bool {
	return true
}

// Left panics (Right has no left value)
func (r *Right[T]) Left() error {
	panic("Right has no left value")
}

// Right returns the value
func (r *Right[T]) Right() T {
	return r.value
}

// Pair represents a pair of values
type Pair[A, B any] struct {
	First  A
	Second B
}

// NewPair creates a new Pair
func NewPair[A, B any](first A, second B) *Pair[A, B] {
	return &Pair[A, B]{
		First:  first,
		Second: second,
	}
}

// Unit represents a unit type (void)
type Unit struct{}

// NewUnit creates a new Unit
func NewUnit() Unit {
	return Unit{}
}
