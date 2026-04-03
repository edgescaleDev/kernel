package sdk

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// TranslatableField stores translated text as a JSON object.
// Stored as JSONB in PostgreSQL: {"en": "Hello", "ar": "مرحبا"}.
// Implements GORM's Scanner/Valuer for seamless read/write.
type TranslatableField map[string]string

// Get returns the translation for the given locale.
// Falls back to "en" if the locale is not found, then to the first available.
func (t TranslatableField) Get(locale string) string {
	if v, ok := t[locale]; ok {
		return v
	}
	if v, ok := t["en"]; ok {
		return v
	}
	// Return first available.
	for _, v := range t {
		return v
	}
	return ""
}

// Scan implements sql.Scanner for reading JSONB from PostgreSQL.
func (t *TranslatableField) Scan(value any) error {
	if value == nil {
		*t = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("TranslatableField.Scan: expected []byte, got %T", value)
	}
	return json.Unmarshal(bytes, t)
}

// Value implements driver.Valuer for writing JSONB to PostgreSQL.
func (t TranslatableField) Value() (driver.Value, error) {
	if t == nil {
		return nil, nil
	}
	return json.Marshal(t)
}
