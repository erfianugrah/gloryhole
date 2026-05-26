package policy

import (
	"context"
	"sync"
	"testing"

	"glory-hole/pkg/storage"
)

// fakeProfileStorage implements just enough of storage.Storage to drive
// the SQLiteResolver — namely ListClientProfiles. All other methods are
// inherited from NoOpStorage. The profiles slice is mutable under mu.
type fakeProfileStorage struct {
	storage.NoOpStorage
	mu       sync.Mutex
	profiles []*storage.ClientProfile
}

func (f *fakeProfileStorage) ListClientProfiles(_ context.Context) ([]*storage.ClientProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*storage.ClientProfile, len(f.profiles))
	copy(out, f.profiles)
	return out, nil
}

func (f *fakeProfileStorage) set(profiles []*storage.ClientProfile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles = profiles
}

// resetResolver restores the package-level resolver to the noop default
// after a test mutates it. Pair with t.Cleanup.
func resetResolver(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		var n ClientGroupResolver = noopResolver{}
		resolver.Store(&n)
	})
}

func TestNoopResolver_AlwaysFalse(t *testing.T) {
	r := noopResolver{}
	if r.IsInGroup("10.0.0.1", "kids") {
		t.Fatal("noopResolver.IsInGroup must always return false")
	}
	if r.IsInGroup("", "") {
		t.Fatal("noopResolver.IsInGroup must always return false (empty inputs)")
	}
}

func TestInClientGroup_DefaultIsNoop(t *testing.T) {
	resetResolver(t)
	if InClientGroup("10.0.0.1", "kids") {
		t.Fatal("default resolver must be noop — InClientGroup returned true unexpectedly")
	}
}

func TestSetClientGroupResolver_NilFallsBackToNoop(t *testing.T) {
	resetResolver(t)
	// Set a real resolver, then nil it — must fall back to noop.
	stor := &fakeProfileStorage{profiles: []*storage.ClientProfile{
		{ClientIP: "10.0.0.1", GroupName: "kids"},
	}}
	r := NewSQLiteResolver(stor)
	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	SetClientGroupResolver(r)
	if !InClientGroup("10.0.0.1", "kids") {
		t.Fatal("expected real resolver to report 10.0.0.1 in 'kids'")
	}

	SetClientGroupResolver(nil)
	if InClientGroup("10.0.0.1", "kids") {
		t.Fatal("after SetClientGroupResolver(nil), InClientGroup must return false")
	}
}

func TestSQLiteResolver_BuildAndQuery(t *testing.T) {
	stor := &fakeProfileStorage{
		profiles: []*storage.ClientProfile{
			{ClientIP: "10.0.0.1", GroupName: "kids"},
			{ClientIP: "10.0.0.2", GroupName: "kids"},
			{ClientIP: "10.0.0.3", GroupName: "iot"},
			{ClientIP: "10.0.0.4", GroupName: ""}, // unprofiled-for-groups
		},
	}
	r := NewSQLiteResolver(stor)
	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	cases := []struct {
		ip, group string
		want      bool
	}{
		{"10.0.0.1", "kids", true},
		{"10.0.0.2", "kids", true},
		{"10.0.0.3", "iot", true},
		{"10.0.0.1", "iot", false},
		{"10.0.0.3", "kids", false},
		{"10.0.0.4", "kids", false}, // empty group name skipped on build
		{"10.0.0.4", "iot", false},
		{"10.0.0.99", "kids", false}, // unknown IP
		{"10.0.0.1", "", false},      // empty group query
	}
	for _, tc := range cases {
		if got := r.IsInGroup(tc.ip, tc.group); got != tc.want {
			t.Errorf("IsInGroup(%q, %q) = %v, want %v", tc.ip, tc.group, got, tc.want)
		}
	}
}

func TestSQLiteResolver_ReloadAfterMutation(t *testing.T) {
	stor := &fakeProfileStorage{
		profiles: []*storage.ClientProfile{
			{ClientIP: "10.0.0.1", GroupName: "kids"},
		},
	}
	r := NewSQLiteResolver(stor)
	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("initial reload: %v", err)
	}
	if !r.IsInGroup("10.0.0.1", "kids") {
		t.Fatal("initial state: expected 10.0.0.1 in 'kids'")
	}

	// Mutate underlying storage: move 10.0.0.1 to 'iot', add 10.0.0.2 to 'kids'.
	stor.set([]*storage.ClientProfile{
		{ClientIP: "10.0.0.1", GroupName: "iot"},
		{ClientIP: "10.0.0.2", GroupName: "kids"},
	})

	// Cache is stale until Reload.
	if !r.IsInGroup("10.0.0.1", "kids") {
		t.Fatal("pre-reload: cache must still report old state")
	}

	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("reload after mutation: %v", err)
	}

	if r.IsInGroup("10.0.0.1", "kids") {
		t.Error("post-reload: 10.0.0.1 should no longer be in 'kids'")
	}
	if !r.IsInGroup("10.0.0.1", "iot") {
		t.Error("post-reload: 10.0.0.1 should now be in 'iot'")
	}
	if !r.IsInGroup("10.0.0.2", "kids") {
		t.Error("post-reload: 10.0.0.2 should now be in 'kids'")
	}
}

func TestSQLiteResolver_NilStorage(t *testing.T) {
	r := NewSQLiteResolver(nil)
	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("Reload with nil storage must not error, got: %v", err)
	}
	if r.IsInGroup("10.0.0.1", "kids") {
		t.Fatal("nil-storage resolver must return false")
	}
}

func TestSQLiteResolver_NilReceiver(t *testing.T) {
	var r *SQLiteResolver
	if r.IsInGroup("10.0.0.1", "kids") {
		t.Fatal("nil receiver IsInGroup must return false (defensive)")
	}
}

// TestSQLiteResolver_ConcurrentReadDuringReload exercises the race detector
// against a Reload happening alongside hot-path IsInGroup reads. Run under
// `go test -race`.
func TestSQLiteResolver_ConcurrentReadDuringReload(t *testing.T) {
	stor := &fakeProfileStorage{
		profiles: []*storage.ClientProfile{
			{ClientIP: "10.0.0.1", GroupName: "kids"},
		},
	}
	r := NewSQLiteResolver(stor)
	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("initial reload: %v", err)
	}

	const readers = 8
	const iters = 1000

	var readersWG sync.WaitGroup
	for i := 0; i < readers; i++ {
		readersWG.Add(1)
		go func() {
			defer readersWG.Done()
			for j := 0; j < iters; j++ {
				_ = r.IsInGroup("10.0.0.1", "kids")
				_ = r.IsInGroup("10.0.0.2", "kids")
				_ = r.IsInGroup("10.0.0.3", "iot")
			}
		}()
	}

	stop := make(chan struct{})
	var reloaderWG sync.WaitGroup
	reloaderWG.Add(1)
	go func() {
		defer reloaderWG.Done()
		variants := [][]*storage.ClientProfile{
			{{ClientIP: "10.0.0.1", GroupName: "kids"}},
			{{ClientIP: "10.0.0.1", GroupName: "iot"}, {ClientIP: "10.0.0.2", GroupName: "kids"}},
			{},
			{{ClientIP: "10.0.0.3", GroupName: "kids"}},
		}
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			stor.set(variants[i%len(variants)])
			_ = r.Reload(context.Background())
			i++
		}
	}()

	readersWG.Wait()
	close(stop)
	reloaderWG.Wait()
}

// TestInClientGroup_EndToEndThroughEngine verifies the full pipeline:
// register the DSL helper at compile time, install a real resolver, build a
// rule that invokes InClientGroup, evaluate against a Context. Catches
// regressions in helper registration or evaluation env.
func TestInClientGroup_EndToEndThroughEngine(t *testing.T) {
	resetResolver(t)

	stor := &fakeProfileStorage{profiles: []*storage.ClientProfile{
		{ClientIP: "10.0.0.50", GroupName: "kids"},
	}}
	r := NewSQLiteResolver(stor)
	if err := r.Reload(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	SetClientGroupResolver(r)

	engine := NewEngine(nil)
	rule := &Rule{
		Name:    "block-kids-from-example",
		Logic:   `Domain == "example.com" && InClientGroup(ClientIP, "kids")`,
		Enabled: true,
		Action:  ActionBlock,
	}
	if err := engine.AddRule(rule); err != nil {
		t.Fatalf("AddRule: %v", err)
	}

	cases := []struct {
		name    string
		ctx     Context
		wantHit bool
	}{
		{"kid hits target", Context{Domain: "example.com", ClientIP: "10.0.0.50"}, true},
		{"non-kid hits target", Context{Domain: "example.com", ClientIP: "10.0.0.99"}, false},
		{"kid hits unrelated", Context{Domain: "other.com", ClientIP: "10.0.0.50"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, _ := engine.Evaluate(tc.ctx)
			if matched != tc.wantHit {
				t.Errorf("Evaluate matched=%v, want %v (ctx=%+v)", matched, tc.wantHit, tc.ctx)
			}
		})
	}
}

// BenchmarkInClientGroup measures the hot-path cost of the DSL primitive on
// a 10k-profile cache. Target: <100 ns/op and 0 allocs/op.
func BenchmarkInClientGroup(b *testing.B) {
	resetResolver(&testing.T{}) // ensure clean state
	defer func() {
		var n ClientGroupResolver = noopResolver{}
		resolver.Store(&n)
	}()

	const profileCount = 10_000
	profiles := make([]*storage.ClientProfile, 0, profileCount)
	for i := 0; i < profileCount; i++ {
		group := "iot"
		if i%10 == 0 {
			group = "kids"
		}
		profiles = append(profiles, &storage.ClientProfile{
			ClientIP:  ipForIndex(i),
			GroupName: group,
		})
	}
	stor := &fakeProfileStorage{profiles: profiles}
	r := NewSQLiteResolver(stor)
	if err := r.Reload(context.Background()); err != nil {
		b.Fatalf("reload: %v", err)
	}
	SetClientGroupResolver(r)

	hitIP := ipForIndex(50) // exists, in "iot"
	missIP := "192.0.2.99"  // never in cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			_ = InClientGroup(hitIP, "iot")
		} else {
			_ = InClientGroup(missIP, "iot")
		}
	}
}

// ipForIndex returns a deterministic 10.x.y.z address for benchmark seeding.
func ipForIndex(i int) string {
	a := byte((i >> 16) & 0xff)
	b := byte((i >> 8) & 0xff)
	c := byte(i & 0xff)
	return "10." + itoa(a) + "." + itoa(b) + "." + itoa(c)
}

func itoa(b byte) string {
	const digits = "0123456789"
	if b < 10 {
		return string([]byte{digits[b]})
	}
	if b < 100 {
		return string([]byte{digits[b/10], digits[b%10]})
	}
	return string([]byte{digits[b/100], digits[(b/10)%10], digits[b%10]})
}
