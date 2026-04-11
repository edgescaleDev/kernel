package kernel

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"go.edgescale.dev/kernel/internal"
	"go.edgescale.dev/kernel/sdk"
)

// ─── validateSchema ───────────────────────────────────────────────────────────

func TestValidateSchema_Valid(t *testing.T) {
	valid := []string{
		"public",
		"billing",
		"my_schema",
		"Schema1",
		"_private",
		"a",
	}
	for _, name := range valid {
		if err := validateSchema(name); err != nil {
			t.Errorf("validateSchema(%q) returned error: %v", name, err)
		}
	}
}

func TestValidateSchema_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"bad schema",
		"has-hyphen",
		"inject; DROP TABLE",
		"a.b",
		"1starts_with_digit",
		// exceeds PostgreSQL's NAMEDATALEN-1 (63 chars)
		"this_schema_name_is_way_too_long_for_postgres_and_exceeds_63_char",
	}
	for _, name := range invalid {
		if err := validateSchema(name); err == nil {
			t.Errorf("validateSchema(%q) should return error but did not", name)
		}
	}
}

// ─── CollectMigrationFiles ────────────────────────────────────────────────────

func TestCollectMigrationFiles_SortedOrder(t *testing.T) {
	memFS := fstest.MapFS{
		"003_third.up.sql":  &fstest.MapFile{Data: []byte("-- v3")},
		"001_first.up.sql":  &fstest.MapFile{Data: []byte("-- v1")},
		"002_second.up.sql": &fstest.MapFile{Data: []byte("-- v2")},
		"001_first.down.sql": &fstest.MapFile{Data: []byte("-- d1")},
	}

	files, err := internal.CollectMigrationFiles(memFS)
	if err != nil {
		t.Fatalf("CollectMigrationFiles: %v", err)
	}

	want := []string{"001_first.up.sql", "002_second.up.sql", "003_third.up.sql"}
	if len(files) != len(want) {
		t.Fatalf("CollectMigrationFiles count = %d, want %d", len(files), len(want))
	}
	for i, f := range want {
		if files[i] != f {
			t.Errorf("files[%d] = %q, want %q", i, files[i], f)
		}
	}
}

func TestCollectMigrationFiles_Empty(t *testing.T) {
	memFS := fstest.MapFS{}
	files, err := internal.CollectMigrationFiles(memFS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no files, got %v", files)
	}
}

// ─── CollectDownFiles ─────────────────────────────────────────────────────────

func TestCollectDownFiles(t *testing.T) {
	memFS := fstest.MapFS{
		"001_init.up.sql":   &fstest.MapFile{Data: []byte("-- up")},
		"001_init.down.sql": &fstest.MapFile{Data: []byte("-- down")},
		"002_add.up.sql":    &fstest.MapFile{Data: []byte("-- up2")},
	}

	downFiles, err := internal.CollectDownFiles(memFS)
	if err != nil {
		t.Fatalf("CollectDownFiles: %v", err)
	}

	if !downFiles["001_init.down.sql"] {
		t.Error("expected 001_init.down.sql to be in downFiles")
	}
	if downFiles["002_add.up.sql"] {
		t.Error("up files should not be in downFiles")
	}
	if len(downFiles) != 1 {
		t.Errorf("downFiles count = %d, want 1", len(downFiles))
	}
}

// ─── Rollback (no DB) ─────────────────────────────────────────────────────────

func TestRollback_InvalidSteps(t *testing.T) {
	k := New(DefaultConfig())

	for _, steps := range []int{0, -1, -100} {
		err := k.Rollback("billing", steps)
		if err == nil {
			t.Errorf("Rollback(steps=%d) should return error", steps)
		}
	}
}

func TestRollback_UnregisteredModule(t *testing.T) {
	k := New(DefaultConfig())

	err := k.Rollback("nonexistent", 1)
	if err == nil {
		t.Fatal("Rollback with unregistered module should return error")
	}
}

func TestRollback_ModuleWithNoMigrations(t *testing.T) {
	k := New(DefaultConfig())
	k.MustRegister(newStub("empty")) // Migrations() returns nil

	err := k.Rollback("empty", 1)
	if err == nil {
		t.Fatal("Rollback on module with no migrations should return error")
	}
}

func TestRollback_InvalidSchema(t *testing.T) {
	k := New(DefaultConfig())
	mod := &stubMigrationModule{
		id:     "badschema",
		schema: "bad-schema!", // invalid characters
		fsData: fstest.MapFS{
			"001_init.up.sql": &fstest.MapFile{Data: []byte("-- up")},
		},
	}
	k.MustRegister(mod)

	err := k.Rollback("badschema", 1)
	if err == nil {
		t.Fatal("Rollback with invalid schema name should return error")
	}
}

// ─── stubMigrationModule ──────────────────────────────────────────────────────

// stubMigrationModule is a module that provides a custom migration FS and schema.
type stubMigrationModule struct {
	id     string
	schema string
	fsData fs.FS
}

func (s *stubMigrationModule) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:      s.id,
		Name:    s.id,
		Version: "1.0.0",
		Type:    sdk.TypeFeature,
		Schema:  s.schema,
	}
}

func (s *stubMigrationModule) Migrations() fs.FS        { return s.fsData }
func (s *stubMigrationModule) Init(_ sdk.Context) error { return nil }
func (s *stubMigrationModule) Shutdown() error          { return nil }
