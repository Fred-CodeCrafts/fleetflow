package auth

import (
	"context"
	"sync"

	"fleetflow/internal/domain"
)

// Overlay is a Repository that answers from a set of keys held in memory and
// delegates everything else to the store behind it.
//
// It exists for one caller: the process's own simulator. A run authenticates
// through the real middleware, like any other client, but its credential must not
// be a secret somebody has to issue, persist, rotate and keep out of the
// repository. Minting it at startup and keeping it here means there is nothing to
// leak from a database dump and nothing to clean up after a restart, while the
// request still takes the same path as an external one — a bypass would be a hole
// in a system others use.
//
// Only the digest of each key is held, exactly as the store would hold it, so an
// overlay key cannot be read back out of this process either.
type Overlay struct {
	next Repository

	mu     sync.RWMutex
	byHash map[string]Key
}

// NewOverlay returns an Overlay over next, holding extra. Keys are indexed by
// their digest, so verification costs one map lookup. A nil next is allowed: the
// overlay then serves only its own keys, which is what a database-free run wants.
func NewOverlay(next Repository, extra ...Key) *Overlay {
	o := &Overlay{next: next, byHash: make(map[string]Key, len(extra))}
	for _, k := range extra {
		o.byHash[string(k.Hash)] = k
	}
	return o
}

// Create stores k in the underlying repository. The overlay adds no keys at
// runtime: its own are fixed when it is built, because a key that could be minted
// through the API would be a key that could be minted by anyone who reached it.
func (o *Overlay) Create(ctx context.Context, k Key) (Key, error) {
	if o.next == nil {
		return Key{}, domain.Internal("api_key_store_unavailable", "no api key store is configured")
	}
	return o.next.Create(ctx, k)
}

// ByHash answers from the overlay first, then the store. A miss in both is the
// same KindNotFound a plain repository would report, so the service turns it into
// ErrUnauthenticated without knowing an overlay is involved.
func (o *Overlay) ByHash(ctx context.Context, hash []byte) (Key, error) {
	o.mu.RLock()
	k, ok := o.byHash[string(hash)]
	o.mu.RUnlock()
	if ok {
		return k, nil
	}

	if o.next == nil {
		return Key{}, domain.NotFound("api_key_not_found", "no api key with this hash")
	}
	return o.next.ByHash(ctx, hash)
}
