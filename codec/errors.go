package codec

import "errors"

var (
	InvalidDataErr   = errors.New("invalid input data")
	InvalidPrefixErr = errors.New("invalid data prefix")
	PrefixReadErr    = errors.New("prefix read error")
)
