package mcpaccess

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestOperationCapacityReservesEveryReceipt(t *testing.T) {
	dir := t.TempDir()
	s := OpenOperations(dir)
	args := json.RawMessage(`{"value":"` + strings.Repeat("a", MaxOperationArguments-12) + `"}`)
	result := json.RawMessage(`{"value":"` + strings.Repeat("b", MaxOperationResult-12) + `"}`)
	for i := 0; i < MaxOperations; i++ {
		o := Operation{ID: fmt.Sprintf("%032x", i+1), ClientID: strings.Repeat("a", 32), Key: fmt.Sprintf("request-%d", i), HostID: "local", HostIdentity: strings.Repeat("b", 64), Tool: "test", Arguments: args, Result: result, State: "succeeded", CreatedAt: time.Now(), UpdatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
		o.Digest = operationDigest(o)
		s.state.Items = append(s.state.Items, o)
	}
	last := &s.state.Items[MaxOperations-1]
	last.State = "approved"
	last.Result = nil
	if err := s.save(s.state); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(last.ID, last.ClientID, last.Digest, last.HostIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finish(last.ID, "succeeded", result, ""); err != nil {
		t.Fatal("receipt budget was not reserved", err)
	}
	if _, err := s.Plan(last.ClientID, "one-too-many", "local", last.HostIdentity, "test", json.RawMessage(`{}`), false); !errors.Is(err, ErrLimited) {
		t.Fatal(err)
	}
	items, err := OpenOperations(dir).List("")
	if err != nil || len(items) != MaxOperations {
		t.Fatal("capacity rejection damaged existing records", len(items), err)
	}
	if _, err := s.Get(last.ID); err != nil {
		t.Fatal(err)
	}
}
