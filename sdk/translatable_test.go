package sdk

import (
	"testing"
)

func TestTranslatableField_Get(t *testing.T) {
	tf := TranslatableField{"en": "Hello", "ar": "مرحبا"}

	if got := tf.Get("en"); got != "Hello" {
		t.Errorf("Get(en) = %q, want %q", got, "Hello")
	}
	if got := tf.Get("ar"); got != "مرحبا" {
		t.Errorf("Get(ar) = %q, want %q", got, "مرحبا")
	}
}

func TestTranslatableField_FallbackToEnglish(t *testing.T) {
	tf := TranslatableField{"en": "Hello", "fr": "Bonjour"}

	if got := tf.Get("de"); got != "Hello" {
		t.Errorf("Get(de) should fall back to en, got %q", got)
	}
}

func TestTranslatableField_FallbackToFirst(t *testing.T) {
	tf := TranslatableField{"fr": "Bonjour"}

	got := tf.Get("de")
	if got != "Bonjour" {
		t.Errorf("Get(de) should fall back to first available, got %q", got)
	}
}

func TestTranslatableField_Empty(t *testing.T) {
	tf := TranslatableField{}
	if got := tf.Get("en"); got != "" {
		t.Errorf("empty field Get(en) = %q, want empty", got)
	}
}

func TestTranslatableField_ScanValue(t *testing.T) {
	original := TranslatableField{"en": "Hello", "ar": "مرحبا"}

	val, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}

	var scanned TranslatableField
	if err := scanned.Scan(val); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if scanned.Get("en") != "Hello" {
		t.Errorf("roundtrip en = %q, want %q", scanned.Get("en"), "Hello")
	}
	if scanned.Get("ar") != "مرحبا" {
		t.Errorf("roundtrip ar = %q, want %q", scanned.Get("ar"), "مرحبا")
	}
}

func TestTranslatableField_ScanNil(t *testing.T) {
	var tf TranslatableField
	if err := tf.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if tf != nil {
		t.Error("Scan(nil) should set to nil")
	}
}

func TestTranslatableField_ValueNil(t *testing.T) {
	var tf TranslatableField
	val, err := tf.Value()
	if err != nil {
		t.Fatalf("Value() nil error: %v", err)
	}
	if val != nil {
		t.Errorf("nil field Value() = %v, want nil", val)
	}
}

func TestTranslatableField_Constructors(t *testing.T) {
	tf1 := T("Hello", "ar", "Ahlan")
	if tf1["en"] != "Hello" || tf1["ar"] != "Ahlan" {
		t.Errorf("T() mismatch: %v", tf1)
	}

	tf2 := Translations("en", "Hi", "es", "Hola")
	if len(tf2) != 2 || tf2["en"] != "Hi" || tf2["es"] != "Hola" {
		t.Errorf("Translations() mismatch: %v", tf2)
	}
}

func TestTranslatableField_SetMerge(t *testing.T) {
	tf := TranslatableField{}
	tf = tf.Set("ar", "مرحبا").Set("fr", "Bonjour")

	if tf.Get("fr") != "Bonjour" {
		t.Errorf("Set() failed")
	}

	tf2 := T("Hello", "es", "Hola")
	tf = tf.Merge(tf2)

	if tf.Get("en") != "Hello" || tf.Get("ar") != "مرحبا" || tf.Get("es") != "Hola" {
		t.Errorf("Merge() failed: %v", tf)
	}
}

func TestTranslatableField_QueryHelpers(t *testing.T) {
	tf := T("Hi", "es", "Hola")

	if !tf.Has("es") || tf.Has("fr") {
		t.Errorf("Has() failed")
	}

	locales := tf.Locales()
	if len(locales) != 2 {
		t.Errorf("Locales() failed: %v", locales)
	}

	if tf.String() != "Hi" {
		t.Errorf("String() failed")
	}
}
