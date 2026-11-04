package apperr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMappingWrap(t *testing.T) {
	errMissing := errors.New("missing")
	m := Mapping{errMissing: NotFound(CodeNotFound, "thing not found")}

	var got *Error
	if !errors.As(m.Wrap(fmt.Errorf("load: %w", errMissing)), &got) || got.Type != TypeNotFound {
		t.Fatalf("Wrap(wrapped sentinel) = %v, want NotFound", got)
	}
	if !errors.Is(got, errMissing) {
		t.Fatal("mapped error should keep the sentinel as its cause")
	}
	if !errors.As(m.Wrap(errors.New("boom")), &got) || got.Code != CodeInternal {
		t.Fatalf("Wrap(unknown) = %v, want Internal", got)
	}
	if m.Wrap(nil) != nil {
		t.Fatal("Wrap(nil) should be nil")
	}
}

func TestInternalClassifiesDatabaseErrorsByType(t *testing.T) {
	if got := Internal(fmt.Errorf("query: %w", &pgconn.PgError{Code: "57014"})); got.Code != CodeDatabase {
		t.Errorf("Internal(PgError).Code = %s, want %s", got.Code, CodeDatabase)
	}
	// A message mentioning "connection" is not a database error any more.
	if got := Internal(errors.New("smtp connection refused")); got.Code != CodeInternal {
		t.Errorf("Internal(non-db).Code = %s, want %s", got.Code, CodeInternal)
	}
}
