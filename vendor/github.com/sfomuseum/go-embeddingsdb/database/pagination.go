package database

import (
	"fmt"
)

type PaginationType uint8

const NullPaginationTypeLabel string = "null"

const CountablePaginationTypeLabel string = "countable"

const CursorPaginationTypeLabel string = "cursor"

const (
	NullPaginationType PaginationType = iota
	CountablePaginationType
	CursorPaginationType
)

func (p PaginationType) String() string {

	switch p {
	case NullPaginationType:
		return NullPaginationTypeLabel
	case CountablePaginationType:
		return CountablePaginationTypeLabel
	case CursorPaginationType:
		return CursorPaginationTypeLabel
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
	default:
		return NullPaginationType, fmt.Errorf("Invalid pagination label")
	}
}
