package backupremote

import (
	"bytes"
	"errors"
	"github.com/kejilion/kejilion-panel/internal/backup"
	"os"
	"path/filepath"
	"testing"
	"time"
	_ "time/tzdata"
)

func TestPriorWorkerCannotOverwriteChangedSchedule(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	revision := s.Snapshot().Revision
	p := s.Plan()
	p.Hour = 4
	if err := s.PutSchedule(revision, p, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunErrorForRevision(revision, "failed"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale worker=%v", err)
	}
	if s.Plan().LastError != "" {
		t.Fatal("stale error leaked into the new schedule")
	}
}

func TestClaimBindsConfigurationRevision(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, revision := s.PlanWithRevision()
	plan.Enabled, plan.Password = true, "long-password"
	if err := s.PutSchedule(revision, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	plan, revision = s.PlanWithRevision()
	claimed, claimedRevision, run, err := s.ClaimWithRevision(plan.NextRun, false)
	if err != nil || !run || claimedRevision != revision || claimed.Password != plan.Password {
		t.Fatalf("claim=%+v revision=%s run=%v err=%v", claimed.Public(), claimedRevision, run, err)
	}
	plan.Hour = (plan.Hour + 1) % 24
	if err := s.PutSchedule(revision, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunErrorForRevision(claimedRevision, "start_failed"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale start failure=%v", err)
	}
}

func testStorage() Storage {
	return Storage{ID: backup.NewID(), Name: "NAS", Kind: "webdav", Endpoint: "https://nas.example/dav", Prefix: "kpanel", Username: "backup", Secret: "secret-never-returned"}
}
func TestEncryptedSettingsRestartAndRevision(t *testing.T) {
	root := t.TempDir()
	s, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	input := testStorage()
	input.ID = ""
	revision := s.Snapshot().Revision
	if err = s.PutStorage(revision, input); err != nil {
		t.Fatal(err)
	}
	public := s.Snapshot()
	if public.Storages[0].Secret != "" || !public.Storages[0].HasSecret {
		t.Fatal("credential leaked")
	}
	if s.PutStorage(revision, input) != ErrConflict {
		t.Fatal("stale write accepted")
	}
	p := public.Schedule
	p.Enabled = true
	p.Password = "archive-passphrase"
	p.StorageID = public.Storages[0].ID
	if err = s.PutSchedule(public.Revision, p, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "settings.enc"))
	if bytes.Contains(b, []byte(input.Secret)) || bytes.Contains(b, []byte(p.Password)) {
		t.Fatal("plaintext persisted")
	}
	reloaded, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Plan().Password != p.Password || reloaded.Snapshot().Schedule.Password != "" {
		t.Fatal("password persistence contract")
	}
	pub := reloaded.Snapshot()
	changed := pub.Storages[0]
	changed.Name = "rotated label"
	if err = reloaded.PutStorage(pub.Revision, changed); err != nil {
		t.Fatal(err)
	}
	private, _ := reloaded.Storage(changed.ID)
	if private.Secret != input.Secret {
		t.Fatal("blank overwrote credential")
	}
	if reloaded.DeleteStorage(reloaded.Snapshot().Revision, changed.ID) != ErrConflict {
		t.Fatal("in-use target removed")
	}
	if err = reloaded.Pause(); err != nil {
		t.Fatal(err)
	}
	if reloaded.Plan().Enabled {
		t.Fatal("restore did not pause")
	}
	b[len(b)-1] ^= 1
	os.WriteFile(filepath.Join(root, "settings.enc"), b, 0600)
	if _, err = OpenStore(root); err == nil {
		t.Fatal("tampered settings accepted")
	}
}

func TestScheduleDueRestartBusyAndCalendar(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	p := s.Plan()
	p.Enabled = true
	p.Password = "long-password"
	p.Timezone = "Asia/Shanghai"
	p.Hour = 3
	now := time.Date(2026, 9, 28, 18, 59, 0, 0, time.UTC)
	if err := s.PutSchedule(s.Snapshot().Revision, p, now); err != nil {
		t.Fatal(err)
	}
	due := s.Plan().NextRun
	if !due.Equal(time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC)) {
		t.Fatal(due)
	}
	if _, run, _ := s.Claim(due, true); run {
		t.Fatal("ran while busy")
	}
	if _, run, err := s.Claim(due.Add(time.Minute), false); err != nil || !run {
		t.Fatal(run, err)
	}
	s2, err := OpenStore(s.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, run, _ := s2.Claim(due.Add(time.Minute), false); run {
		t.Fatal("duplicate after restart")
	}
	if _, run, _ := s2.Claim(s2.Plan().NextRun.Add(6*time.Minute), false); run || s2.Plan().LastError != "schedule_missed" {
		t.Fatal("missed slot replayed")
	}
	p.Frequency = "monthly"
	p.Day = 31
	p.Timezone = "UTC"
	if got := p.Next(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)); got.Month() != time.May || got.Day() != 31 {
		t.Fatal(got)
	}
	p.Frequency = "daily"
	p.Timezone = "America/New_York"
	p.Hour = 2
	p.Minute = 30
	if got := p.Next(time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)); got.Day() != 9 {
		t.Fatal("DST gap", got)
	}
	p.Hour = 1
	first := p.Next(time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC))
	second := p.Next(first)
	loc, _ := time.LoadLocation(p.Timezone)
	if second.In(loc).Day() != 2 {
		t.Fatal("DST repeated", first, second)
	}
}

func TestStorageAndObjectBoundaries(t *testing.T) {
	for _, endpoint := range []string{"https://user:pass@host/dav", "https://host/dav?secret=x", "https://host/dav#x", "file:///etc", "https://host/a/../dav", "https://host/%2e%2e/dav", "https://host:99999/dav"} {
		v := testStorage()
		v.Endpoint = endpoint
		if v.Validate() == nil {
			t.Fatal("accepted", endpoint)
		}
	}
	for _, key := range []string{"../secret.kpb", "a/b.kpb", "x%2fk.kpb", "x.kpb?secret", "x.kpb\x00", "x\\a.kpb"} {
		if ValidObject(key) {
			t.Fatal(key)
		}
	}
}
