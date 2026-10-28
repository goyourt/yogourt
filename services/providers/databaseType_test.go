package providers

import (
	"strings"
	"testing"
)

func TestValidateDatabaseTypeAcceptsPostgresAndSilence(t *testing.T) {
	for _, databaseType := range []string{"", "postgres", "postgresql", " PostgreSQL ", "POSTGRES"} {
		if err := validateDatabaseType(databaseType); err != nil {
			t.Errorf("validateDatabaseType(%q) = %v, want nil", databaseType, err)
		}
	}
}

func TestValidateDatabaseTypeRejectsAnotherDriver(t *testing.T) {
	for _, databaseType := range []string{"mysql", "sqlite", "mongo"} {
		err := validateDatabaseType(databaseType)
		if err == nil {
			t.Fatalf("validateDatabaseType(%q) = nil, want an error", databaseType)
		}
		if !strings.Contains(err.Error(), databaseType) {
			t.Errorf("the error should name the rejected value, got %v", err)
		}
	}
}
