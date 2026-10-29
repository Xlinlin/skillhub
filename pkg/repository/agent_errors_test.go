package repository

import (
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"net"
	"syscall"
	"testing"
)

func TestAgentDBErrorClassification(t *testing.T) {
	for _, code := range []string{"23505", "40001", "40P01"} {
		if e := agentDBError(&pgconn.PgError{Code: code}); !errors.Is(e, ErrAgentConflict) {
			t.Fatal(code, e)
		}
	}
	original := &pgconn.PgError{Code: "22000"}
	if e := agentDBError(original); e != original {
		t.Fatal("unknown error turned into conflict", e)
	}
	e := agentDBError(&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED})
	if !errors.Is(e, ErrAgentUnavailable) {
		t.Fatal("connection failure must be unavailable", e)
	}
}
