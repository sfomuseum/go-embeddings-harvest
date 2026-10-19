package database

import (
	"fmt"
)

type PaginationType uint8

const NullPaginationTypeLabel string = "null"

const CountablePaginationTypeLabel string = "countable"

const CursorPaginationTypeLabel string = "cursor"

const UndefinedPaginationTypeLabel string = ""

const MultiPaginationTypeLabel string = "multi"

const (
	NullPaginationType PaginationType = iota
	CountablePaginationType
	CursorPaginationType
	UndefinedPaginationType
	MultiPaginationType
)

func (p PaginationType) String() string {

	switch p {
	case NullPaginationType:
		return NullPaginationTypeLabel
	case CountablePaginationType:
		return CountablePaginationTypeLabel
	case CursorPaginationType:
		return CursorPaginationTypeLabel
	case MultiPaginationType:
		return MultiPaginationTypeLabel
	default:
		return ""
	}
}

func NewPaginationType(label string) (PaginationType, error) {

	switch label {
	case NullPaginationTypeLabel:
		return NullPaginationType, nil
	case CountablePaginationTypeLabel:
		return CountablePaginationType, nil
	case CursorPaginationTypeLabel:
		return CursorPaginationType, nil
	case MultiPaginationTypeLabel:
		return MultiPaginationType, nil
	default:
		return NullPaginationType, fmt.Errorf("Invalid pagination label")
	}
}
