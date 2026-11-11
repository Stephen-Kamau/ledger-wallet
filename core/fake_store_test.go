package core

import (
	"context"
	"sort"
	"sync"
)

// fakeStore is an in-memory core.Store for engine unit tests. It proves the engine
// enforces R1-R8 without a database: a map-backed Store and Tx exercise every rule.
// InTx serializes writers behind a mutex, mirroring the SQLite single-writer model
// (§5) closely enough for the invariant tests (true contention is Kofi's sqlite-level
// suite).
type fakeStore struct {
	mu        sync.Mutex
	accounts  map[AccountID]Account
	transfers []Transfer
	postings  []Posting
}

func newFakeStore() *fakeStore {
	return &fakeStore{accounts: map[AccountID]Account{}}
}

func (f *fakeStore) InTx(ctx context.Context, fn func(tx Tx) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fn(&fakeTx{s: f})
}

func (f *fakeStore) GetAccount(ctx context.Context, id AccountID) (Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getAccount(id)
}

func (f *fakeStore) getAccount(id AccountID) (Account, error) {
	a, ok := f.accounts[id]
	if !ok {
		return Account{}, ErrAccountNotFound
	}
	return a, nil
}

func (f *fakeStore) AccountsByOwner(ctx context.Context, owner PrincipalID) ([]Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Account
	for _, a := range f.accounts {
		if a.Owner == owner {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeStore) Balance(ctx context.Context, id AccountID) (Amount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.getAccount(id); err != nil {
		return 0, err
	}
	return f.balance(id), nil
}

func (f *fakeStore) balance(id AccountID) Amount {
	var sum Amount
	for _, p := range f.postings {
		if p.Account == id {
			sum += p.Amount
		}
	}
	return sum
}

func (f *fakeStore) Statement(ctx context.Context, id AccountID, page Page) (StatementPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.getAccount(id); err != nil {
		return StatementPage{}, err
	}
	var ps []Posting
	for _, p := range f.postings {
		if p.Account == id && p.Seq > page.AfterSeq {
			ps = append(ps, p)
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Seq < ps[j].Seq })
	out := StatementPage{NextAfterSeq: page.AfterSeq}
	if len(ps) > page.Limit {
		out.HasMore = true
		ps = ps[:page.Limit]
	}
	out.Postings = ps
	if n := len(ps); n > 0 {
		out.NextAfterSeq = ps[n-1].Seq
	}
	return out, nil
}

// fakeTx mutates the parent store directly; InTx already holds the lock.
type fakeTx struct{ s *fakeStore }

func (t *fakeTx) GetAccount(id AccountID) (Account, error) { return t.s.getAccount(id) }
func (t *fakeTx) Balance(id AccountID) (Amount, error)     { return t.s.balance(id), nil }

func (t *fakeTx) InsertAccount(a Account) error {
	t.s.accounts[a.ID] = a
	return nil
}

func (t *fakeTx) FindTransferByKey(owner PrincipalID, key string) (Transfer, bool, error) {
	for _, tr := range t.s.transfers {
		if tr.Owner == owner && tr.IdempotencyKey == key {
			return tr, true, nil
		}
	}
	return Transfer{}, false, nil
}

func (t *fakeTx) InsertTransfer(tr Transfer) error {
	// Assign seq as MAX(seq)+1 per account, mirroring the adapter (D-B1).
	for i := range tr.Postings {
		var max int64
		for _, p := range t.s.postings {
			if p.Account == tr.Postings[i].Account && p.Seq > max {
				max = p.Seq
			}
		}
		tr.Postings[i].Seq = max + 1
		t.s.postings = append(t.s.postings, tr.Postings[i])
	}
	t.s.transfers = append(t.s.transfers, tr)
	return nil
}
