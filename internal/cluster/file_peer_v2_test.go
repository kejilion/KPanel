package cluster

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testFilePeerStoreV2(t *testing.T) (*filePeerStoreV2, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), filePeerStateV2FileName)
	store, err := openFilePeerStoreV2(path)
	if err != nil {
		t.Fatalf("openFilePeerStoreV2() error = %v", err)
	}
	return store, path
}

func testFilePeerHostV2(seed byte, now time.Time) hostRecordV2 {
	record := testHostRecordV2(seed, now)
	record.Scope = SummaryTerminalFilesScope
	record.ResourceVersion = hostResourceVersionV2(record)
	return record
}

func testFilePeerPendingHostV2(
	seed byte,
	state hostStateV2,
	now time.Time,
) hostRecordV2 {
	record := testFilePeerHostV2(seed, now)
	record.State = state
	record.PairingCredentialFile = fmt.Sprintf("pair-%016x.v2key", int(seed)+200)
	if state == hostStateV2PendingPair {
		record.CredentialFile = ""
		record.Scope = SummaryScope
	}
	record.ResourceVersion = hostResourceVersionV2(record)
	return record
}

func testFilePeerControllerV2(seed byte, now time.Time) controllerRecordV2 {
	record := testControllerRecordV2(
		seed,
		fmt.Sprintf("%032x", int(seed)+180),
		now,
	)
	record.Scope = SummaryTerminalFilesScope
	record.State = controllerStateV2Active
	return record
}

func TestFilePeerStoreV2EmptyStoreReopensWithArrays(t *testing.T) {
	_, path := testFilePeerStoreV2(t)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte(`"schemaVersion": 1`)) ||
		!bytes.Contains(content, []byte(`"routes": []`)) ||
		!bytes.Contains(content, []byte(`"grants": []`)) {
		t.Fatalf("empty file peer state did not persist arrays:\n%s", content)
	}
	if _, err := openFilePeerStoreV2(path); err != nil {
		t.Fatalf("reopen empty file peer store: %v", err)
	}
}

func TestFilePeerStoreV2GrantLifecycleAndNonDisruptiveRefresh(t *testing.T) {
	store, path := testFilePeerStoreV2(t)
	now := time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC)
	host := testFilePeerHostV2(1, now)

	grant, err := store.PrepareGrant(host, "https://center.example", now)
	if err != nil {
		t.Fatalf("PrepareGrant() error = %v", err)
	}
	if grant.State != filePeerGrantPending ||
		!grant.NextSynchronization.Equal(now) || !grant.ExpiresAt.IsZero() {
		t.Fatalf("new grant = %+v, want due pending grant", grant)
	}
	if _, err := store.ActiveGrant(grant.LinkID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ActiveGrant(pending) error = %v, want ErrNotFound", err)
	}

	pendingContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	samePending, err := store.PrepareGrant(host, grant.LocalOrigin, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("PrepareGrant(same pending) error = %v", err)
	}
	if samePending != grant {
		t.Fatalf("same pending grant changed: got %+v want %+v", samePending, grant)
	}
	afterSamePending, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pendingContent, afterSamePending) {
		t.Fatal("same pending grant rewrote the sidecar")
	}

	activatedAt := now.Add(2 * time.Minute)
	grant, err = store.ActivateGrant(grant.LinkID, activatedAt)
	if err != nil {
		t.Fatalf("ActivateGrant() error = %v", err)
	}
	if grant.State != filePeerGrantActive ||
		!grant.LastSynchronizedAt.Equal(activatedAt) ||
		!grant.NextSynchronization.Equal(activatedAt.Add(filePeerSyncInterval)) ||
		!grant.ExpiresAt.Equal(activatedAt.Add(filePeerLeaseDuration)) {
		t.Fatalf("active grant lease = %+v", grant)
	}

	activeContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sameActive, err := store.PrepareGrant(
		host,
		grant.LocalOrigin,
		activatedAt.Add(5*time.Minute),
	)
	if err != nil {
		t.Fatalf("PrepareGrant(same active) error = %v", err)
	}
	if sameActive != grant {
		t.Fatalf("same active grant changed: got %+v want %+v", sameActive, grant)
	}
	afterSameActive, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(activeContent, afterSameActive) {
		t.Fatal("same active grant rewrote the sidecar")
	}

	refreshed, err := store.PrepareGrant(
		host,
		"https://new-center.example",
		activatedAt.Add(6*time.Minute),
	)
	if err != nil {
		t.Fatalf("PrepareGrant(new origin) error = %v", err)
	}
	if refreshed.State != filePeerGrantActive ||
		refreshed.LocalOrigin != "https://new-center.example" ||
		!refreshed.UpdatedAt.Equal(grant.UpdatedAt) ||
		!refreshed.LastSynchronizedAt.Equal(grant.LastSynchronizedAt) ||
		!refreshed.NextSynchronization.Equal(grant.NextSynchronization) ||
		!refreshed.ExpiresAt.Equal(grant.ExpiresAt) {
		t.Fatalf("origin refresh interrupted active grant: before=%+v after=%+v", grant, refreshed)
	}
	if _, err := store.ActiveGrant(
		grant.LinkID,
		activatedAt.Add(filePeerLeaseDuration-time.Second),
	); err != nil {
		t.Fatalf("active grant was lost before its original lease expired: %v", err)
	}

	reopened, err := openFilePeerStoreV2(path)
	if err != nil {
		t.Fatalf("reopen file peer store: %v", err)
	}
	persisted, err := reopened.ActiveGrantByHost(
		host.ID,
		activatedAt.Add(filePeerLeaseDuration-time.Second),
	)
	if err != nil || persisted.LocalOrigin != refreshed.LocalOrigin {
		t.Fatalf("reopened active grant = %+v, %v", persisted, err)
	}

	expiredAt := activatedAt.Add(filePeerLeaseDuration)
	if _, err := reopened.ActiveGrant(grant.LinkID, expiredAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ActiveGrant(expired) error = %v, want ErrNotFound", err)
	}
	reconciledAt := expiredAt.Add(time.Second)
	if err := reopened.Reconcile(nil, []hostRecordV2{host}, reconciledAt); err != nil {
		t.Fatalf("Reconcile(expired grant) error = %v", err)
	}
	pending, err := reopened.GrantByHost(host.ID)
	if err != nil {
		t.Fatalf("GrantByHost() error = %v", err)
	}
	if pending.State != filePeerGrantPending || !pending.ExpiresAt.IsZero() ||
		!pending.LastSynchronizedAt.IsZero() ||
		!pending.NextSynchronization.Equal(reconciledAt) {
		t.Fatalf("reconciled expired grant = %+v, want due pending", pending)
	}
}

func TestFilePeerStoreV2PendingIntentSurvivesRestartAndActivation(t *testing.T) {
	for index, state := range []hostStateV2{
		hostStateV2PendingPair,
		hostStateV2PendingCommit,
	} {
		t.Run(string(state), func(t *testing.T) {
			store, path := testFilePeerStoreV2(t)
			now := time.Date(2026, 8, 16, 1, 30+index, 0, 0, time.UTC)
			host := testFilePeerPendingHostV2(byte(20+index), state, now)
			grant, err := store.PrepareGrant(host, "https://center.example", now)
			if err != nil {
				t.Fatalf("PrepareGrant(%s) error = %v", state, err)
			}
			if grant.State != filePeerGrantPending {
				t.Fatalf("pending parent grant state = %q", grant.State)
			}
			if _, err := store.ActiveGrant(grant.LinkID, now); !errors.Is(err, ErrNotFound) {
				t.Fatalf("pending intent authorized before activation: %v", err)
			}

			reopened, err := openFilePeerStoreV2(path)
			if err != nil {
				t.Fatalf("reopen pending intent: %v", err)
			}
			if err := reopened.Reconcile(nil, []hostRecordV2{host}, now.Add(time.Minute)); err != nil {
				t.Fatalf("Reconcile(%s) error = %v", state, err)
			}
			persisted, err := reopened.GrantByHost(host.ID)
			if err != nil || persisted.LinkID != grant.LinkID ||
				persisted.State != filePeerGrantPending {
				t.Fatalf("pending intent after restart = %+v, %v", persisted, err)
			}

			activeHost := host
			activeHost.State = hostStateV2Active
			activeHost.CredentialFile = "host-" + activeHost.ID + ".v2key"
			activeHost.PairingCredentialFile = ""
			activeHost.Scope = SummaryTerminalFilesScope
			activeHost.UpdatedAt = now.Add(2 * time.Minute)
			activeHost.ResourceVersion = hostResourceVersionV2(activeHost)
			if err := reopened.Reconcile(
				nil,
				[]hostRecordV2{activeHost},
				now.Add(2*time.Minute),
			); err != nil {
				t.Fatalf("Reconcile(active parent) error = %v", err)
			}
			if _, err := reopened.ActiveGrant(grant.LinkID, now.Add(2*time.Minute)); !errors.Is(err, ErrNotFound) {
				t.Fatalf("intent became active without link acknowledgement: %v", err)
			}
			if _, err := reopened.ActivateGrant(grant.LinkID, now.Add(3*time.Minute)); err != nil {
				t.Fatalf("ActivateGrant(after parent activation) error = %v", err)
			}
			if _, err := reopened.ActiveGrant(grant.LinkID, now.Add(3*time.Minute)); err != nil {
				t.Fatalf("acknowledged active grant unavailable: %v", err)
			}

			pendingAgain := activeHost
			pendingAgain.State = hostStateV2PendingCommit
			pendingAgain.PairingCredentialFile = fmt.Sprintf(
				"pair-%016x.v2key",
				int(20+index)+220,
			)
			pendingAgain.UpdatedAt = now.Add(4 * time.Minute)
			pendingAgain.ResourceVersion = hostResourceVersionV2(pendingAgain)
			if err := reopened.Reconcile(
				nil,
				[]hostRecordV2{pendingAgain},
				now.Add(4*time.Minute),
			); err != nil {
				t.Fatalf("Reconcile(regressed pending parent) error = %v", err)
			}
			regressed, err := reopened.GrantByHost(host.ID)
			if err != nil || regressed.State != filePeerGrantPending {
				t.Fatalf("pending parent retained active authorization: %+v, %v", regressed, err)
			}
		})
	}
}

func TestFilePeerStoreV2ReconcileHostEligibility(t *testing.T) {
	for index, test := range []struct {
		name  string
		state hostStateV2
		scope string
	}{
		{name: "active without files", state: hostStateV2Active, scope: SummaryScope},
		{name: "pending revoke", state: hostStateV2PendingRevoke, scope: SummaryTerminalFilesScope},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, _ := testFilePeerStoreV2(t)
			now := time.Date(2026, 8, 16, 1, 45+index, 0, 0, time.UTC)
			host := testFilePeerPendingHostV2(byte(30+index), hostStateV2PendingPair, now)
			if _, err := store.PrepareGrant(host, "https://center.example", now); err != nil {
				t.Fatal(err)
			}

			host.State = test.state
			host.CredentialFile = "host-" + host.ID + ".v2key"
			host.PairingCredentialFile = ""
			host.Scope = test.scope
			host.UpdatedAt = now.Add(time.Minute)
			host.ResourceVersion = hostResourceVersionV2(host)
			if err := store.Reconcile(nil, []hostRecordV2{host}, now.Add(time.Minute)); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if _, err := store.GrantByHost(host.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("ineligible parent retained mutual intent: %v", err)
			}
			if _, err := store.PrepareGrant(host, "https://center.example", now.Add(2*time.Minute)); !errors.Is(err, ErrAuthentication) {
				t.Fatalf("PrepareGrant(ineligible parent) error = %v, want ErrAuthentication", err)
			}
		})
	}
}

func TestFilePeerStoreV2ParentAndPeerUniqueness(t *testing.T) {
	store, _ := testFilePeerStoreV2(t)
	now := time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC)
	host := testFilePeerHostV2(2, now)
	grant, err := store.PrepareGrant(host, "https://center.example", now)
	if err != nil {
		t.Fatalf("PrepareGrant() error = %v", err)
	}
	same, err := store.PrepareGrant(host, "https://center.example", now)
	if err != nil || same.LinkID != grant.LinkID {
		t.Fatalf("second PrepareGrant() = %+v, %v; want same link", same, err)
	}

	duplicatePeerHost := testFilePeerHostV2(3, now)
	duplicatePeerHost.RemoteNodeID = host.RemoteNodeID
	duplicatePeerHost.ResourceVersion = hostResourceVersionV2(duplicatePeerHost)
	if _, err := store.PrepareGrant(
		duplicatePeerHost,
		"https://other-center.example",
		now,
	); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("PrepareGrant(duplicate peer) error = %v, want ErrDuplicate", err)
	}

	controller := testFilePeerControllerV2(4, now)
	peerNodeID := strings.Repeat("e", 32)
	routeAt := now.Add(time.Minute)
	route, err := store.GrantRoute(
		controller,
		grant.LinkID,
		peerNodeID,
		"https://center.example",
		routeAt,
	)
	if err != nil {
		t.Fatalf("GrantRoute() error = %v", err)
	}
	if route.ControllerID != controller.ID ||
		!route.ExpiresAt.Equal(routeAt.Add(filePeerLeaseDuration)) {
		t.Fatalf("route = %+v", route)
	}
	if active, err := store.ActiveRoute(peerNodeID, routeAt); err != nil || active != route {
		t.Fatalf("ActiveRoute() = %+v, %v", active, err)
	}

	otherController := testFilePeerControllerV2(5, now)
	otherLinkID := strings.Repeat("a", 32)
	if _, err := store.GrantRoute(
		otherController,
		otherLinkID,
		peerNodeID,
		"https://other-center.example",
		routeAt.Add(time.Second),
	); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("GrantRoute(duplicate active peer) error = %v, want ErrDuplicate", err)
	}

	driftedController := controller
	driftedController.TransactionID = strings.Repeat("f", 32)
	if _, err := store.GrantRoute(
		driftedController,
		grant.LinkID,
		peerNodeID,
		"https://center.example",
		routeAt.Add(time.Second),
	); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("GrantRoute(changed parent) error = %v, want ErrIdentityMismatch", err)
	}

	replacementAt := route.ExpiresAt.Add(time.Second)
	replacement, err := store.GrantRoute(
		otherController,
		otherLinkID,
		peerNodeID,
		"https://other-center.example",
		replacementAt,
	)
	if err != nil {
		t.Fatalf("GrantRoute(after lease expiry) error = %v", err)
	}
	if replacement.ControllerID != otherController.ID {
		t.Fatalf("replacement route = %+v", replacement)
	}
	if err := store.DeleteController(otherController.ID); err != nil {
		t.Fatalf("DeleteController() error = %v", err)
	}
	if _, err := store.ActiveRoute(peerNodeID, replacementAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ActiveRoute(after delete) error = %v, want ErrNotFound", err)
	}
	if err := store.DeleteHost(host.ID); err != nil {
		t.Fatalf("DeleteHost() error = %v", err)
	}
	if _, err := store.GrantByHost(host.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GrantByHost(after delete) error = %v, want ErrNotFound", err)
	}
}

func TestFilePeerStoreV2ReconcileRevokedParents(t *testing.T) {
	store, _ := testFilePeerStoreV2(t)
	now := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	host := testFilePeerHostV2(6, now)
	grant, err := store.PrepareGrant(host, "https://center.example", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivateGrant(grant.LinkID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	controller := testFilePeerControllerV2(7, now)
	peerNodeID := strings.Repeat("d", 32)
	if _, err := store.GrantRoute(
		controller,
		grant.LinkID,
		peerNodeID,
		"https://center.example",
		now.Add(time.Minute),
	); err != nil {
		t.Fatal(err)
	}

	if err := store.Reconcile(nil, nil, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, err := store.GrantByHost(host.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GrantByHost(orphan) error = %v, want ErrNotFound", err)
	}
	if _, err := store.ActiveRoute(peerNodeID, now.Add(2*time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ActiveRoute(orphan) error = %v, want ErrNotFound", err)
	}
}

func TestFilePeerStoreV2AtomicWriteRollback(t *testing.T) {
	store, path := testFilePeerStoreV2(t)
	now := time.Date(2026, 8, 16, 4, 0, 0, 0, time.UTC)
	host := testFilePeerHostV2(8, now)
	grant, err := store.PrepareGrant(host, "https://center.example", now)
	if err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected target install failure")
	originalRename := store.ops.rename
	store.ops.rename = func(oldPath, newPath string) error {
		if filepath.Clean(newPath) == filepath.Clean(path) &&
			filepath.Clean(oldPath) != filepath.Clean(path+".previous") {
			return injected
		}
		return originalRename(oldPath, newPath)
	}
	if _, err := store.ActivateGrant(grant.LinkID, now.Add(time.Minute)); !errors.Is(err, injected) {
		t.Fatalf("ActivateGrant() error = %v, want injected failure", err)
	}

	current, err := store.GrantByHost(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != filePeerGrantPending {
		t.Fatalf("in-memory state after failed persist = %s, want pending", current.State)
	}
	reopened, err := openFilePeerStoreV2(path)
	if err != nil {
		t.Fatalf("reopen after failed persist: %v", err)
	}
	persisted, err := reopened.GrantByHost(host.ID)
	if err != nil || persisted.State != filePeerGrantPending {
		t.Fatalf("persisted state after failed write = %+v, %v", persisted, err)
	}
}

func TestFilePeerStoreV2StrictPrivateBackupRecovery(t *testing.T) {
	store, path := testFilePeerStoreV2(t)
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	host := testFilePeerHostV2(9, now)
	grant, err := store.PrepareGrant(host, "https://center.example", now)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"pairingKey",
		"privateKey",
		"controllerPrivate",
		"credentialFile",
		"targetPublicKey",
	} {
		if bytes.Contains(content, []byte(forbidden)) {
			t.Fatalf("sidecar leaked %q: %s", forbidden, content)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("sidecar permissions = %o, want 600", info.Mode().Perm())
		}
	}

	if err := os.WriteFile(path+".previous", content, 0o600); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte(`{"schemaVersion":1,"routes":[],"grants":[],"unknown":true}`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := openFilePeerStoreV2(path)
	if err != nil {
		t.Fatalf("openFilePeerStoreV2(recover backup) error = %v", err)
	}
	if got, err := recovered.GrantByHost(host.ID); err != nil || got.LinkID != grant.LinkID {
		t.Fatalf("recovered grant = %+v, %v", got, err)
	}

	badPath := filepath.Join(t.TempDir(), filePeerStateV2FileName)
	if err := os.WriteFile(badPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openFilePeerStoreV2(badPath); err == nil {
		t.Fatal("openFilePeerStoreV2() accepted unknown JSON field without backup")
	}

	badFingerprint := grant
	badFingerprint.PeerFingerprint = strings.TrimPrefix(
		badFingerprint.PeerFingerprint,
		"sha256:",
	)
	if err := validateFilePeerGrantV2(badFingerprint); err == nil {
		t.Fatal("validateFilePeerGrantV2() accepted fingerprint without sha256 prefix")
	}
}
