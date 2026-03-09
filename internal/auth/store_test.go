package auth

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestConsumeBootstrapOnce(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	s.SaveBootstrap(BootstrapToken{Token: "b1", AgentID: "a1", ContainerID: "c1", CreatedAt: now, ExpiresAt: now.Add(1 * time.Minute)})
	if _, err := s.ConsumeBootstrap("b1", "c1", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeBootstrap("b1", "c1", now); err == nil {
		t.Fatalf("expected second consume to fail")
	}
}

func TestConsumeBootstrapContainerMismatch(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	s.SaveBootstrap(BootstrapToken{Token: "b2", AgentID: "a1", ContainerID: "expected-c", CreatedAt: now, ExpiresAt: now.Add(1 * time.Minute)})
	if _, err := s.ConsumeBootstrap("b2", "wrong-c", now); err == nil {
		t.Fatalf("expected container mismatch to fail")
	}
}

func TestCleanupExpired(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	s.SaveBootstrap(BootstrapToken{Token: "b_exp", AgentID: "a", CreatedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(-1 * time.Minute)})
	s.SaveBootstrap(BootstrapToken{Token: "b_ok", AgentID: "a", CreatedAt: now, ExpiresAt: now.Add(1 * time.Minute)})
	s.SaveSession(SessionToken{Token: "s_exp", AgentID: "a", CreatedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(-1 * time.Minute)})
	s.SaveGrant(PairingGrant{GrantID: "g_exp", AgentID: "a", CreatedAt: now.Add(-3 * time.Hour), LastUsedAt: now.Add(-2 * time.Hour), IdleExpiresAt: now.Add(-1 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})

	rb, rs, rg := s.CleanupExpired(now)
	if rb < 1 || rs < 1 || rg < 1 {
		t.Fatalf("expected cleanup activity, got rb=%d rs=%d rg=%d", rb, rs, rg)
	}
}

func TestPersistAndLoadStoreMaintainsSessionAndRevocation(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "auth-store.json")

	s1 := NewStore()
	s1.SaveGrant(PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(30 * time.Minute), AbsoluteExpiresAt: now.Add(2 * time.Hour)})
	s1.SaveSession(SessionToken{Token: "sess1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)})
	if err := s1.SaveToFile(path); err != nil {
		t.Fatalf("save to file: %v", err)
	}

	s2 := NewStore()
	if err := s2.LoadFromFile(path); err != nil {
		t.Fatalf("load from file: %v", err)
	}
	if _, err := s2.ValidateSession("sess1", now.Add(1*time.Minute)); err != nil {
		t.Fatalf("expected session to remain valid after reload, got %v", err)
	}

	if err := s2.RevokeGrant("g1"); err != nil {
		t.Fatalf("revoke grant: %v", err)
	}
	if err := s2.SaveToFile(path); err != nil {
		t.Fatalf("save revoked state: %v", err)
	}

	s3 := NewStore()
	if err := s3.LoadFromFile(path); err != nil {
		t.Fatalf("reload revoked state: %v", err)
	}
	g, err := s3.GetGrant("g1")
	if err != nil {
		t.Fatalf("get grant after reload: %v", err)
	}
	if !g.Revoked {
		t.Fatalf("expected revoked grant to persist across reload")
	}
}

func TestPersistAndLoadStoreEncrypted(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "auth-store.enc.json")
	key := []byte("this_is_a_test_encryption_key")

	s1 := NewStore()
	s1.SaveGrant(PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(30 * time.Minute), AbsoluteExpiresAt: now.Add(2 * time.Hour)})
	s1.SaveSession(SessionToken{Token: "sess1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)})
	if err := s1.SaveToFileEncrypted(path, key); err != nil {
		t.Fatalf("save encrypted: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read encrypted file: %v", err)
	}
	if bytes.Contains(raw, []byte("sess1")) {
		t.Fatalf("encrypted file should not contain plaintext token")
	}

	s2 := NewStore()
	if err := s2.LoadFromFileEncrypted(path, key); err != nil {
		t.Fatalf("load encrypted: %v", err)
	}
	if _, err := s2.ValidateSession("sess1", now.Add(1*time.Minute)); err != nil {
		t.Fatalf("expected session to remain valid after encrypted reload, got %v", err)
	}
}

func TestLoadEncryptedStoreWithWrongKeyFails(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "auth-store.enc.json")

	s := NewStore()
	s.SaveGrant(PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(30 * time.Minute), AbsoluteExpiresAt: now.Add(2 * time.Hour)})
	if err := s.SaveToFileEncrypted(path, []byte("correct_test_encryption_key")); err != nil {
		t.Fatalf("save encrypted: %v", err)
	}

	if err := s.LoadFromFileEncrypted(path, []byte("wrong_test_encryption_key")); err == nil {
		t.Fatalf("expected wrong encryption key to fail")
	}
}

func TestSaveToFileDoesNotFollowPreexistingTmpSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}

	now := time.Now().UTC()
	s := NewStore()
	s.SaveGrant(PairingGrant{
		GrantID:           "g1",
		AgentID:           "a1",
		CreatedAt:         now,
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(2 * time.Hour),
	})

	dir := t.TempDir()
	path := filepath.Join(dir, "auth-store.json")
	victimPath := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victimPath, []byte("victim"), 0o600); err != nil {
		t.Fatalf("write victim file: %v", err)
	}
	tmpSymlink := path + ".tmp"
	if err := os.Symlink(victimPath, tmpSymlink); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}

	if err := s.SaveToFile(path); err != nil {
		t.Fatalf("save auth store: %v", err)
	}

	victim, err := os.ReadFile(victimPath)
	if err != nil {
		t.Fatalf("read victim file: %v", err)
	}
	if string(victim) != "victim" {
		t.Fatalf("victim file was modified via tmp symlink; got %q", string(victim))
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat persisted file: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("persisted auth store should not be a symlink")
	}
}

func TestSaveToFileEncryptedConcurrentWriters(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "auth-store.enc.json")
	key := []byte("concurrent_auth_store_key_012345")

	s := NewStore()
	s.SaveGrant(PairingGrant{
		GrantID:           "g1",
		AgentID:           "a1",
		CreatedAt:         now,
		LastUsedAt:        now,
		IdleExpiresAt:     now.Add(30 * time.Minute),
		AbsoluteExpiresAt: now.Add(2 * time.Hour),
	})

	const workers = 8
	const iterations = 25
	errCh := make(chan error, workers*iterations)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				s.SaveSession(SessionToken{
					Token:     fmt.Sprintf("sess-%d-%d", worker, i),
					GrantID:   "g1",
					AgentID:   "a1",
					CreatedAt: now,
					ExpiresAt: now.Add(10 * time.Minute),
				})
				if err := s.SaveToFileEncrypted(path, key); err != nil {
					errCh <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent encrypted save failed: %v", err)
	}

	reloaded := NewStore()
	if err := reloaded.LoadFromFileEncrypted(path, key); err != nil {
		t.Fatalf("load encrypted store after concurrent writes: %v", err)
	}
}
