package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestResolvePreservesPublicErrorAndHidesUnknownCause(t *testing.T) {
	err := WithCause(Conflict, errors.New("database detail"))
	got := Resolve(fmt.Errorf("register: %w", err))
	if got.Code != Conflict.Code || got.Message != Conflict.Message || !errors.Is(err, Conflict) {
		t.Fatalf("lost typed error: %+v", got)
	}
	internal := Resolve(errors.New("private database address"))
	if internal.Code != Internal.Code || internal.Message != Internal.Message {
		t.Fatal("unknown error leaked")
	}
}
