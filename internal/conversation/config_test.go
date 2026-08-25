package conversation

import "testing"

func TestParseStoreConfigDefaultsToMemory(t *testing.T) {
	cfg, err := ParseStoreConfig(map[string]string{})
	if err != nil {
		t.Fatalf("ParseStoreConfig returned error: %v", err)
	}
	if cfg.Backend != "memory" || cfg.MaxMessages != 20 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestParseStoreConfigRequiresPostgresDSN(t *testing.T) {
	if _, err := ParseStoreConfig(map[string]string{"backend": "postgres"}); err == nil {
		t.Fatal("postgres backend without DSN should fail")
	}
}

func TestParseStoreConfigNormalizesMessageLimit(t *testing.T) {
	cfg, err := ParseStoreConfig(map[string]string{
		"backend":      "postgres",
		"dsn":          "postgres://localhost/superbizagent",
		"max_messages": "5",
	})
	if err != nil {
		t.Fatalf("ParseStoreConfig returned error: %v", err)
	}
	if cfg.MaxMessages != 4 {
		t.Fatalf("max messages = %d, want 4", cfg.MaxMessages)
	}
}

func TestOpenConfiguredStoreMemoryDoesNotOpenDatabase(t *testing.T) {
	cfg, err := ParseStoreConfig(map[string]string{"backend": "memory"})
	if err != nil {
		t.Fatal(err)
	}
	store, db, err := OpenConfiguredStore(nil, cfg)
	if err != nil {
		t.Fatalf("OpenConfiguredStore returned error: %v", err)
	}
	if store == nil || db != nil {
		t.Fatalf("memory store result = store %T, db %v", store, db)
	}
}
