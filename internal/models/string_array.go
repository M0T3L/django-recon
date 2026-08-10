package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// StringArray represents a slice of strings stored as a JSON string in the database.
type StringArray []string

// Value implements the driver.Valuer interface for GORM/database write operations.
func (sa StringArray) Value() (driver.Value, error) {
	if len(sa) == 0 {
		return "[]", nil
	}
	bytes, err := json.Marshal(sa)
	if err != nil {
		return nil, err
	}
	return string(bytes), nil
}

// Scan implements the sql.Scanner interface for GORM/database read operations.
func (sa *StringArray) Scan(value interface{}) error {
	if value == nil {
		*sa = StringArray{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("type assertion to []byte or string failed for StringArray")
	}
	if len(bytes) == 0 {
		*sa = StringArray{}
		return nil
	}
	return json.Unmarshal(bytes, sa)
}
