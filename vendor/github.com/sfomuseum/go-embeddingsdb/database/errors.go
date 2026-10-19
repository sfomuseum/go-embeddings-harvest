package database

import (
	"errors"
)

var RecordNotFound = errors.New("Record not found")

var NotImplemented = errors.New("Not implemented")

var NotAvailable = errors.New("Not available")
