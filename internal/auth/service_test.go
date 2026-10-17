package auth_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"

	"fleetflow/internal/auth"
	"fleetflow/internal/auth/memory"
	"fleetflow/internal/domain"
)

// newService returns a Service backed by a fresh in-memory repository, which is
// how DESIGN 3.7 asks services to be unit-tested, plus the store so a test can
// inspect what was actually persisted.
func newService() (*auth.Service, *memory.Repository) {
	repo := memory.New()
	return auth.NewService(repo), repo
}

func TestIssueReturnsPlaintextOnceAndStoresOnlyTheHash(t *testing.T) {
	svc, repo := newService()
	ctx := context.Background()

	plaintext, record, err := svc.Issue(ctx, auth.IssueInput{Name: "dispatch integration"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !strings.HasPrefix(plaintext, auth.KeyPrefix) {
		t.Errorf("Issue returned %q, want the %q prefix", plaintext, auth.KeyPrefix)
	}
	if record.ID == "" {
		t.Error("Issue returned a record with no id")
	}
	if record.Name != "dispatch integration" {
		t.Errorf("Issue name = %q, want the given name", record.Name)
	}
	if !bytes.Equal(record.Hash, auth.Hash(plaintext)) {
		t.Error("Issue stored a hash that is not the hash of the key it returned")
	}
	if record.Prefix != auth.DisplayPrefix(plaintext) {
		t.Errorf("Issue prefix = %q, want the head of the returned key", record.Prefix)
	}
	if record.CreatedAt.IsZero() {
		t.Error("Issue returned a record with no issue time")
	}

	stored, err := repo.ByHash(ctx, auth.Hash(plaintext))
	if err != nil {
		t.Fatalf("ByHash: %v", err)
	}
	if stored.ID != record.ID {
		t.Errorf("stored id = %q, want %q", stored.ID, record.ID)
	}
}

func TestIssueRecordsAreDistinct(t *testing.T) {
	svc, repo := newService()
	ctx := context.Background()

	firstKey, first, err := svc.Issue(ctx, auth.IssueInput{Name: "first"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	secondKey, second, err := svc.Issue(ctx, auth.IssueInput{Name: "second"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if firstKey == secondKey {
		t.Error("Issue returned the same plaintext key twice")
	}
	if first.ID == second.ID {
		t.Error("Issue returned the same id twice")
	}
	if bytes.Equal(first.Hash, second.Hash) {
		t.Error("Issue stored the same hash for two different keys")
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List returned %d keys, want 2", len(list))
	}
}

func TestIssueRequiresNameAndStoresNothing(t *testing.T) {
	for _, name := range []string{"", "   "} {
		svc, repo := newService()
		ctx := context.Background()

		plaintext, record, err := svc.Issue(ctx, auth.IssueInput{Name: name})
		if err == nil {
			t.Fatalf("Issue accepted name %q and returned %q", name, plaintext)
		}
		if plaintext != "" || record.ID != "" {
			t.Errorf("Issue returned %q / %+v alongside an error, want nothing", plaintext, record)
		}
		if kind := domain.KindOf(err); kind != domain.KindInvalid {
			t.Errorf("Issue kind = %s, want invalid", kind)
		}
		if code, _, ok := domain.Public(err); !ok || code != "api_key_name_required" {
			t.Errorf("Issue code = %q (ok=%v), want api_key_name_required", code, ok)
		}

		list, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("a nameless issue persisted %d key(s)", len(list))
		}
	}
}

func TestAuthenticateAcceptsAnIssuedKey(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	plaintext, issued, err := svc.Issue(ctx, auth.IssueInput{Name: "reporting"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := svc.Authenticate(ctx, plaintext)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != issued.ID {
		t.Errorf("Authenticate returned id %q, want %q", got.ID, issued.ID)
	}
}

func TestAuthenticateRejectsUnknownKeyWithOneDomainError(t *testing.T) {
	svc, repo := newService()
	ctx := context.Background()

	plaintext, issued, err := svc.Issue(ctx, auth.IssueInput{Name: "reporting"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	unknown, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	got, err := svc.Authenticate(ctx, unknown)
	if err == nil {
		t.Fatalf("Authenticate accepted the unknown key as %+v", got)
	}
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("Authenticate kind = %s, want not_found", kind)
	}
	if !errors.Is(err, auth.ErrUnauthenticated) {
		t.Error("Authenticate did not return ErrUnauthenticated, so the API cannot map one 401")
	}
	if code, _, ok := domain.Public(err); !ok || code != "unauthenticated" {
		t.Errorf("Authenticate code = %q (ok=%v), want unauthenticated", code, ok)
	}
	if got.ID != "" || got.Hash != nil {
		t.Errorf("Authenticate returned a record %+v alongside its failure, want nothing", got)
	}

	// The failure must not quote the plaintext or the stored digest, in hex or
	// base64, which would hand an attacker the very material it must guess.
	message := err.Error()
	for _, secret := range []string{
		plaintext,
		hex.EncodeToString(issued.Hash),
		base64.StdEncoding.EncodeToString(issued.Hash),
	} {
		if strings.Contains(message, secret) {
			t.Errorf("Authenticate leaked secret material in %q", message)
		}
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("a failed authenticate changed the store: %d key(s)", len(list))
	}
}

func TestAuthenticateRejectsMalformedAndEmptyKeysWithoutPanicking(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	if _, _, err := svc.Issue(ctx, auth.IssueInput{Name: "reporting"}); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	presented := []string{
		"",
		" ",
		auth.KeyPrefix,
		"not-a-key",
		"flf_",
		"flf_\n\t",
		strings.Repeat("f", 4096),
		"flf_" + strings.Repeat("!", 40),
	}

	for _, key := range presented {
		t.Run(key, func(t *testing.T) {
			got, err := svc.Authenticate(ctx, key)
			if err == nil {
				t.Fatalf("Authenticate accepted malformed key %q as %+v", key, got)
			}
			if !errors.Is(err, auth.ErrUnauthenticated) {
				t.Errorf("Authenticate(%q) error = %v, want ErrUnauthenticated", key, err)
			}
		})
	}
}

func TestAuthenticateRejectsEveryWrongKeyIdentically(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	plaintext, _, err := svc.Issue(ctx, auth.IssueInput{Name: "reporting"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	other, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wrong := []string{
		"",                                      // missing entirely
		other,                                   // a valid key that was never issued
		plaintext[:len(plaintext)-1],            // a truncated key
		plaintext + "a",                         // an over-long key
		auth.DisplayPrefix(plaintext) + "wrong", // the label without the secret
	}

	for _, key := range wrong {
		got, err := svc.Authenticate(ctx, key)
		if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("Authenticate(%q) error = %v, want ErrUnauthenticated", key, err)
		}
		if got.ID != "" {
			t.Errorf("Authenticate(%q) returned record %+v, want nothing", key, got)
		}
		// Every rejection must be the same value, so a client (or an attacker)
		// cannot tell "no key", "wrong key" and "truncated key" apart.
		if err != auth.ErrUnauthenticated {
			t.Errorf("Authenticate(%q) returned a different error value %v", key, err)
		}
	}
}

func TestStoredRecordsNeverContainThePlaintext(t *testing.T) {
	svc, repo := newService()
	ctx := context.Background()

	plaintext, _, err := svc.Issue(ctx, auth.IssueInput{Name: "reporting"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d keys, want 1", len(list))
	}

	// Walk every field of the persisted record, so a field added later cannot
	// quietly start carrying plaintext.
	value := reflect.ValueOf(list[0])
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		name := value.Type().Field(i).Name
		switch field.Kind() {
		case reflect.String:
			if strings.Contains(field.String(), plaintext) {
				t.Errorf("stored field Key.%s contains the plaintext key", name)
			}
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.Uint8 && bytes.Contains(field.Bytes(), []byte(plaintext)) {
				t.Errorf("stored field Key.%s contains the plaintext key", name)
			}
		}
	}

	if bytes.Equal(list[0].Hash, []byte(plaintext)) {
		t.Error("the stored hash is the plaintext key")
	}
}

func TestAuthenticateDoesNotTrustALooseRepositoryMatch(t *testing.T) {
	// A store that returns a record for a hash it was not asked about must still
	// not authenticate: the service re-checks in constant time.
	loose := stubRepo{byHashKey: auth.Key{ID: "k1", Name: "real", Hash: auth.Hash("the-real-key")}}
	svc := auth.NewService(loose)

	got, err := svc.Authenticate(context.Background(), "a-different-key")
	if !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("Authenticate error = %v, want ErrUnauthenticated", err)
	}
	if got.ID != "" {
		t.Errorf("Authenticate returned %+v on a loose match, want nothing", got)
	}
}

func TestAuthenticateTreatsAStoreOutageAsUnavailableNotUnauthenticated(t *testing.T) {
	raw := errors.New("pq: dial tcp 10.0.0.7:5432: connection refused")
	svc := auth.NewService(stubRepo{byHashErr: raw})

	_, err := svc.Authenticate(context.Background(), "flf_anything")
	if err == nil {
		t.Fatal("Authenticate succeeded although the repository failed")
	}
	if errors.Is(err, auth.ErrUnauthenticated) {
		t.Error("Authenticate reported an outage as a bad key, which would answer 401 to a live client")
	}
	if kind := domain.KindOf(err); kind != domain.KindInternal {
		t.Errorf("Authenticate kind = %s, want internal", kind)
	}
	code, message, ok := domain.Public(err)
	if !ok || code != "api_key_lookup_failed" {
		t.Errorf("Authenticate code = %q (ok=%v), want api_key_lookup_failed", code, ok)
	}
	if strings.Contains(message, "10.0.0.7") {
		t.Errorf("Authenticate leaked the internal cause in %q", message)
	}
	if !errors.Is(err, raw) {
		t.Error("Authenticate dropped the cause, so logs cannot show why lookup failed")
	}
}

func TestAuthenticateReportsCancelledRequestAsUnavailable(t *testing.T) {
	svc, _ := newService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Authenticate(ctx, "flf_anything")
	if err == nil {
		t.Fatal("Authenticate succeeded with a cancelled context")
	}
	if errors.Is(err, auth.ErrUnauthenticated) {
		t.Error("Authenticate reported cancellation as a bad key")
	}
	if kind := domain.KindOf(err); kind != domain.KindUnavailable {
		t.Errorf("Authenticate kind = %s, want unavailable", kind)
	}
	if code, _, _ := domain.Public(err); code != "request_cancelled" {
		t.Errorf("Authenticate code = %q, want request_cancelled", code)
	}
}

func TestIssueReportsCancelledRequestAsUnavailable(t *testing.T) {
	svc, _ := newService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := svc.Issue(ctx, auth.IssueInput{Name: "reporting"})
	if err == nil {
		t.Fatal("Issue succeeded with a cancelled context")
	}
	if kind := domain.KindOf(err); kind != domain.KindUnavailable {
		t.Errorf("Issue kind = %s, want unavailable", kind)
	}
	if code, _, _ := domain.Public(err); code != "request_cancelled" {
		t.Errorf("Issue code = %q, want request_cancelled", code)
	}
}

// stubRepo is an auth.Repository whose results a test dictates, so the service's
// classification can be checked without a database.
type stubRepo struct {
	byHashKey auth.Key
	byHashErr error
	createErr error
}

func (s stubRepo) Create(context.Context, auth.Key) (auth.Key, error) {
	return auth.Key{}, s.createErr
}

func (s stubRepo) ByHash(context.Context, []byte) (auth.Key, error) {
	if s.byHashErr != nil {
		return auth.Key{}, s.byHashErr
	}
	return s.byHashKey, nil
}
