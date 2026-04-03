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
