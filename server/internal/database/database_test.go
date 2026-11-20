package database

import (
	"context"
	"testing"
)

func TestMigrateAcceptsEmptyMigrationDirectory(t *testing.T) {
	foundMigrations, err := Migrate(context.Background(), nil, t.TempDir())
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if foundMigrations {
		t.Fatal("expected an empty directory to contain no schema migrations")
	}
}
