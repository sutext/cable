package result

type Result[V any] struct {
	err   error
	value V
}

func OK[V any](value V) Result[V] {
	return Result[V]{value: value, err: nil}
}
func Err[V any](err error) Result[V] {
	var v V
	return Result[V]{value: v, err: err}
}

func (r Result[V]) Value() V {
	return r.value
}
func (r Result[V]) Error() error {
	return r.err
}

func (r Result[V]) IsOk() bool {
	return r.err == nil
}
func (r Result[V]) IsErr() bool {
	return r.err != nil
}
