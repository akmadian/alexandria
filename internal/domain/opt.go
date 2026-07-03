package domain

// Opt is a three-state type for sparse patch updates:
// Set=false means "don't touch", Set=true with Value=nil means "clear", Set=true with non-nil Value means "set".
type Opt[T any] struct {
	Set   bool
	Value *T
}

func SetOpt[T any](v T) Opt[T] {
	return Opt[T]{Set: true, Value: &v}
}

func ClearOpt[T any]() Opt[T] {
	return Opt[T]{Set: true, Value: nil}
}
