package auth_test

import (
	"context"
	"testing"

	"fleetflow/internal/auth"
	authmemory "fleetflow/internal/auth/memory"
	"fleetflow/internal/domain"
)

// overlayKey returns a stored record for plaintext, the way the process builds one
// for its simulator: the digest and nothing else.
func overlayKey(plaintext string) auth.Key {
	return auth.Key{
		ID:     "simulator",
		Name:   "simulator",
		Prefix: auth.DisplayPrefix(plaintext),
		Hash:   auth.Hash(plaintext),
	}
}

// TestOverlay_AuthenticatesItsOwnKey is the property the simulator depends on: the
// minted key is accepted through the ordinary service, with no bypass.
func TestOverlay_AuthenticatesItsOwnKey(t *testing.T) {
	const plaintext = "flf_an_ephemeral_simulator_key"

	svc := auth.NewService(auth.NewOverlay(authmemory.New(), overlayKey(plaintext)))

	got, err := svc.Authenticate(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.Name != "simulator" {
		t.Errorf("Name = %q, want simulator", got.Name)
	}
}

// TestOverlay_StillRefusesEverythingElse keeps the overlay from becoming a hole: it
// adds one key, it does not accept arbitrary ones.
func TestOverlay_StillRefusesEverythingElse(t *testing.T) {
	svc := auth.NewService(auth.NewOverlay(authmemory.New(), overlayKey("flf_the_real_one")))

	for _, presented := range []string{"flf_not_the_real_one", "", "flf_the_real_on"} {
		if _, err := svc.Authenticate(context.Background(), presented); err == nil {
			t.Errorf("Authenticate accepted %q", presented)
		}
	}
}

// TestOverlay_DelegatesToTheStoreUnderneath proves the overlay is not a replacement
// for the database: a key issued through the service is found in the store behind
// it, and the overlay's own key is found without touching it.
func TestOverlay_DelegatesToTheStoreUnderneath(t *testing.T) {
	store := authmemory.New()
	svc := auth.NewService(auth.NewOverlay(store, overlayKey("flf_ephemeral")))

	issued, _, err := svc.Issue(context.Background(), auth.IssueInput{Name: "dispatch"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := svc.Authenticate(context.Background(), issued)
	if err != nil {
		t.Fatalf("Authenticate issued key: %v", err)
	}
	if got.Name != "dispatch" {
		t.Errorf("Name = %q, want dispatch", got.Name)
	}
}

// TestOverlay_WithoutAStoreHoldsOnlyItsOwnKeys covers a database-free run: the
// overlay is the whole store, so anything it was not built with is refused rather
// than reported as a storage failure.
func TestOverlay_WithoutAStoreHoldsOnlyItsOwnKeys(t *testing.T) {
	overlay := auth.NewOverlay(nil, overlayKey("flf_ephemeral"))

	if _, err := overlay.ByHash(context.Background(), auth.Hash("flf_ephemeral")); err != nil {
		t.Fatalf("ByHash for its own key: %v", err)
	}

	_, err := overlay.ByHash(context.Background(), auth.Hash("flf_something_else"))
	if kind := domain.KindOf(err); kind != domain.KindNotFound {
		t.Errorf("ByHash kind = %s, want not found", kind)
	}
}
