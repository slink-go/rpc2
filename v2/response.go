package rpc2

type Response[T any] struct {
	Error  error
	Status int
	Data   T
}
