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
	if t == nil {
		return ""
	}
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

// T creates a TranslatableField. First arg is the "en" value.
// Optional subsequent args are locale, value pairs.
// Panics if an odd number of pairs is provided (missing value for a locale).
func T(en string, pairs ...string) TranslatableField {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("sdk.T: odd number of pairs (%d); each locale must have a value", len(pairs)))
	}
	t := TranslatableField{"en": en}
	for i := 0; i < len(pairs); i += 2 {
		t[pairs[i]] = pairs[i+1]
	}
	return t
}

// Translations creates a TranslatableField from locale, value pairs.
// No default locale assumed.
// Panics if an odd number of pairs is provided (missing value for a locale).
func Translations(pairs ...string) TranslatableField {
	if len(pairs)%2 != 0 {
		panic(fmt.Sprintf("sdk.Translations: odd number of pairs (%d); each locale must have a value", len(pairs)))
	}
	t := TranslatableField{}
	for i := 0; i < len(pairs); i += 2 {
		t[pairs[i]] = pairs[i+1]
	}
	return t
}

// Set adds or overwrites a single locale.
func (t TranslatableField) Set(locale, value string) TranslatableField {
	if t == nil {
		t = TranslatableField{}
	}
	t[locale] = value
	return t
}

// Merge combines another TranslatableField into this one.
// Existing locales are overwritten.
func (t TranslatableField) Merge(other TranslatableField) TranslatableField {
	if t == nil {
		t = TranslatableField{}
	}
	for k, v := range other {
		t[k] = v
	}
	return t
}

// Has returns true if the given locale exists and is non-empty.
func (t TranslatableField) Has(locale string) bool {
	if t == nil {
		return false
	}
	v, ok := t[locale]
	return ok && v != ""
}

// Locales returns all locale codes present.
func (t TranslatableField) Locales() []string {
	if t == nil {
		return nil
	}
	var locales []string
	for k := range t {
		locales = append(locales, k)
	}
	return locales
}

// String returns the "en" value or first available (for fmt.Stringer).
func (t TranslatableField) String() string {
	return t.Get("en")
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
