package runner

import (
	"bytes"
	"encoding/json"
	"errors"
)

var ErrEmptyLine = errors.New("empty line")

// Parser defines a generic interface for parsing line bytes into a concrete type T.
type Parser[T any] interface {
	Parse(line []byte) (T, error)
}

// JSONParser is a generic implementation of Parser that unmarshals JSON/JSONL lines into type T.
type JSONParser[T any] struct{}

// NewJSONParser initializes a new JSONParser for type T.
func NewJSONParser[T any]() *JSONParser[T] {
	return &JSONParser[T]{}
}

// Parse unmarshals a single JSON/JSONL byte slice into target struct type T.
func (p *JSONParser[T]) Parse(line []byte) (T, error) {
	var item T
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return item, ErrEmptyLine
	}

	if err := json.Unmarshal(trimmed, &item); err != nil {
		return item, err
	}

	return item, nil
}

// Mapper defines a generic interface to map an intermediate parsed type TIn into a target DB model TOut.
type Mapper[TIn any, TOut any] interface {
	Map(input TIn) (TOut, error)
}

// BuildLineHandler creates a LineHandler callback by pairing a Parser[T] with a model consumer function.
// Blank or whitespace-only lines are automatically skipped.
func BuildLineHandler[T any](parser Parser[T], itemHandler func(item T) error) LineHandler {
	return func(line []byte) error {
		item, err := parser.Parse(line)
		if err != nil {
			if errors.Is(err, ErrEmptyLine) {
				return nil // skip blank lines gracefully
			}
			return err
		}
		return itemHandler(item)
	}
}

// BuildMappedLineHandler creates a LineHandler that parses TIn, maps it to TOut using Mapper, and passes TOut to itemHandler.
func BuildMappedLineHandler[TIn any, TOut any](
	parser Parser[TIn],
	mapper Mapper[TIn, TOut],
	itemHandler func(item TOut) error,
) LineHandler {
	return func(line []byte) error {
		rawItem, err := parser.Parse(line)
		if err != nil {
			if errors.Is(err, ErrEmptyLine) {
				return nil
			}
			return err
		}

		mappedItem, err := mapper.Map(rawItem)
		if err != nil {
			return err
		}

		return itemHandler(mappedItem)
	}
}
