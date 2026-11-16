package api

import (
	"context"
	"sort"
	"time"

	"github.com/Stephen-Kamau/ledger-wallet/core"
)

// memStore is an in-memory core.Store used to drive the REAL core engine from the
// api tests (so ownership/R7, insufficient funds/R3 and idempotency replay/R4 are
// exercised through core.New, not re-implemented). Serial tests only; no locking.
type memStore struct {
	accounts   map[core.AccountID]core.Account
	postings   []core.Posting
	transfers  map[string]core.Transfer // key = owner + "\x00" + idempotencyKey
	seq        map[core.AccountID]int64
	currencies []core.CurrencyMeta
	holds      map[core.HoldID]core.Hold
}

func newMemStore() *memStore {
	return &memStore{
		accounts:  map[core.AccountID]core.Account{},
		transfers: map[string]core.Transfer{},
		seq:       map[core.AccountID]int64{},
		holds:     map[core.HoldID]core.Hold{},
	}
}

func (m *memStore) ActiveHoldsTotal(ctx context.Context, id core.AccountID) (core.Amount, error) {
	if _, err := m.GetAccount(ctx, id); err != nil {
		return 0, err
	}
	return m.activeHoldsTotal(id), nil
}

func (m *memStore) activeHoldsTotal(id core.AccountID) core.Amount {
	var sum core.Amount
	for _, h := range m.holds {
		if h.Account == id && h.Active {
			sum += h.Amount
		}
	}
	return sum
}

func (m *memStore) InTx(ctx context.Context, fn func(tx core.Tx) error) error {
	return fn(&memTx{m: m})
}

func (m *memStore) GetAccount(ctx context.Context, id core.AccountID) (core.Account, error) {
	a, ok := m.accounts[id]
	if !ok {
		return core.Account{}, core.ErrAccountNotFound
	}
	return a, nil
}

func (m *memStore) AccountsByOwner(ctx context.Context, owner core.PrincipalID) ([]core.Account, error) {
	var out []core.Account
	for _, a := range m.accounts {
		if a.Owner == owner {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *memStore) Balance(ctx context.Context, id core.AccountID) (core.Amount, error) {
	if _, err := m.GetAccount(ctx, id); err != nil {
		return 0, err
	}
	return m.balanceOf(id), nil
}

func (m *memStore) Statement(ctx context.Context, id core.AccountID, page core.Page) (core.StatementPage, error) {
	if _, err := m.GetAccount(ctx, id); err != nil {
		return core.StatementPage{}, err
	}
	limit := page.Limit
	if limit <= 0 {
		limit = 50
	}
	var got []core.Posting
	for _, p := range m.postings {
		if p.Account == id && p.Seq > page.AfterSeq {
			got = append(got, p)
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Seq < got[j].Seq })

	out := core.StatementPage{}
	if len(got) > limit {
		out.HasMore = true
		got = got[:limit]
	}
	out.Postings = got
	if n := len(got); n > 0 {
		out.NextAfterSeq = got[n-1].Seq
	} else {
		out.NextAfterSeq = page.AfterSeq
	}
	return out, nil
}

func (m *memStore) ListCurrencies(ctx context.Context) ([]core.CurrencyMeta, error) {
	out := make([]core.CurrencyMeta, len(m.currencies))
	copy(out, m.currencies)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (m *memStore) balanceOf(id core.AccountID) core.Amount {
	var sum core.Amount
	for _, p := range m.postings {
		if p.Account == id {
			sum += p.Amount
		}
	}
	return sum
}

// memTx is the transactional surface over memStore.
type memTx struct{ m *memStore }

func (t *memTx) GetAccount(id core.AccountID) (core.Account, error) {
	return t.m.GetAccount(context.Background(), id)
}

func (t *memTx) Balance(id core.AccountID) (core.Amount, error) {
	return t.m.balanceOf(id), nil
}

func (t *memTx) InsertAccount(a core.Account) error {
	t.m.accounts[a.ID] = a
	return nil
}

func (t *memTx) FindTransferByKey(owner core.PrincipalID, key string) (core.Transfer, bool, error) {
	tr, ok := t.m.transfers[string(owner)+"\x00"+key]
	return tr, ok, nil
}

func (t *memTx) InsertTransfer(tr core.Transfer) error {
	// Assign seq as MAX(seq)+1 per account (D-B1), mirroring the SQLite adapter.
	for i := range tr.Postings {
		acct := tr.Postings[i].Account
		t.m.seq[acct]++
		tr.Postings[i].Seq = t.m.seq[acct]
	}
	t.m.postings = append(t.m.postings, tr.Postings...)
	t.m.transfers[string(tr.Owner)+"\x00"+tr.IdempotencyKey] = tr
	return nil
}

func (t *memTx) ActiveHoldsTotal(id core.AccountID) (core.Amount, error) {
	return t.m.activeHoldsTotal(id), nil
}

func (t *memTx) FindHoldByKey(owner core.PrincipalID, key string) (core.Hold, bool, error) {
	for _, h := range t.m.holds {
		if h.Owner == owner && h.IdempotencyKey == key {
			return h, true, nil
		}
	}
	return core.Hold{}, false, nil
}

func (t *memTx) InsertHold(h core.Hold) error {
	t.m.holds[h.ID] = h
	return nil
}

func (t *memTx) GetHold(id core.HoldID) (core.Hold, bool, error) {
	h, ok := t.m.holds[id]
	return h, ok, nil
}

func (t *memTx) ReleaseHold(id core.HoldID) error {
	if h, ok := t.m.holds[id]; ok {
		h.Active = false
		t.m.holds[id] = h
	}
	return nil
}

// fakeLedger is a programmable core.Ledger for precise error-mapping and D-D1
// tests. Each method returns the configured value/error.
type fakeLedger struct {
	account     core.Account
	accountErr  error
	accounts    []core.Account
	accountsErr error
	balance     core.Amount
	balanceErr  error
	result      core.TransferResult
	transferErr error
	statement     core.StatementPage
	staterr       error
	created       core.Account
	createErr     error
	currencies    []core.CurrencyMeta
	currenciesErr error
	hold          core.Hold
	holdErr       error
	releaseErr    error
	available     core.Amount
	availErr      error
}

func (f *fakeLedger) CreateAccount(ctx context.Context, cmd core.CreateAccountCmd) (core.Account, error) {
	if f.createErr != nil {
		return core.Account{}, f.createErr
	}
	acc := f.created
	if acc.ID == "" {
		acc = core.Account{ID: "acct_fake", Owner: cmd.Owner, Currency: cmd.Currency,
			AllowNegative: cmd.AllowNegative, CreatedAt: time.Unix(0, 0).UTC()}
	}
	return acc, nil
}

func (f *fakeLedger) GetAccount(ctx context.Context, id core.AccountID) (core.Account, error) {
	return f.account, f.accountErr
}

func (f *fakeLedger) AccountsByOwner(ctx context.Context, owner core.PrincipalID) ([]core.Account, error) {
	return f.accounts, f.accountsErr
}

func (f *fakeLedger) Balance(ctx context.Context, id core.AccountID) (core.Amount, error) {
	return f.balance, f.balanceErr
}

func (f *fakeLedger) Transfer(ctx context.Context, cmd core.TransferCmd) (core.TransferResult, error) {
	return f.result, f.transferErr
}

func (f *fakeLedger) Statement(ctx context.Context, id core.AccountID, page core.Page) (core.StatementPage, error) {
	return f.statement, f.staterr
}

func (f *fakeLedger) ListCurrencies(ctx context.Context) ([]core.CurrencyMeta, error) {
	return f.currencies, f.currenciesErr
}

func (f *fakeLedger) PlaceHold(ctx context.Context, cmd core.HoldCmd) (core.Hold, error) {
	return f.hold, f.holdErr
}

func (f *fakeLedger) ReleaseHold(ctx context.Context, authorizer core.PrincipalID, id core.HoldID) error {
	return f.releaseErr
}

func (f *fakeLedger) AvailableBalance(ctx context.Context, id core.AccountID) (core.Amount, error) {
	return f.available, f.availErr
}
