package domains

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AloofLibra/zator/internal/orchestra"
	"github.com/AloofLibra/zator/internal/paths"
)

func newMgr(t *testing.T) (*Manager, paths.Paths) {
	t.Helper()
	p := paths.New(t.TempDir(), t.TempDir())
	return New(p), p
}

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in      string
		dot     bool
		want    string
		wantErr bool
	}{
		{"Example.COM", true, "example.com", false},
		{"  example.com  ", true, "example.com", false},
		{"https://www.YouTube.com/watch?v=1", true, "www.youtube.com", false},
		{"example.com:8443/path", true, "example.com", false},
		{"user:pw@host.example.org/x", true, "host.example.org", false},
		{"example.com.", true, "example.com", false},
		{"localhost", false, "localhost", false},
		{"localhost", true, "", true},
		{"", true, "", true},
		{"bad domain.com", true, "", true},
		{"-bad.com", true, "", true},
		{"exa_mple.com", true, "", true},
		{"https://", true, "", true},
		{strings.Repeat("a", 254) + ".com", true, "", true},
	}
	for _, c := range cases {
		got, err := NormalizeDomain(c.in, c.dot)
		if c.wantErr {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("NormalizeDomain(%q) err = %v, want ErrInvalid", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestNormalizeSubstring(t *testing.T) {
	// Регистр сохраняется (контракт Web UI / TestDomainsSubstring).
	if got, err := NormalizeSubstring("  Tube "); err != nil || got != "Tube" {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := NormalizeSubstring("a b"); err != nil || got != "a b" {
		t.Fatalf("space substring: got %q, %v", got, err)
	}
	for _, bad := range []string{"", "   ", "#comment"} {
		if _, err := NormalizeSubstring(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("NormalizeSubstring(%q) err = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestKindMetadata(t *testing.T) {
	want := map[Kind]string{
		KindNetrogat:           "netrogat.txt",
		KindNetrogatSubstrings: "netrogat_substrings.txt",
		KindCustom:             "TCP_Custom.txt",
		KindRKNSubstrings:      "TCP_RKN_domains_by_substring.txt",
	}
	for k, name := range want {
		if k.FileName() != name {
			t.Errorf("%s FileName = %q, want %q", k, k.FileName(), name)
		}
	}
	if Kind("nope").FileName() != "" {
		t.Error("unknown kind must have empty file name")
	}
	if len(AllKinds()) != 4 {
		t.Errorf("AllKinds = %v", AllKinds())
	}
	if !KindNetrogatSubstrings.IsSubstring() || KindNetrogat.IsSubstring() {
		t.Error("IsSubstring")
	}
	if !KindCustom.HasStrategies() || KindNetrogat.HasStrategies() {
		t.Error("HasStrategies")
	}
}

func TestPathsUnderListsDir(t *testing.T) {
	m, p := newMgr(t)
	for _, k := range AllKinds() {
		path, err := m.Path(k)
		if err != nil {
			t.Fatal(err)
		}
		wantDir := p.ListsDir
		if k == KindCustom || k == KindRKNSubstrings {
			wantDir = p.ExtraStratsDir
		}
		if filepath.Dir(path) != wantDir {
			t.Errorf("%s: %s not under %s", k, path, wantDir)
		}
	}
	if _, err := m.Path("bogus"); !errors.Is(err, ErrInvalid) {
		t.Errorf("Path(bogus) err = %v", err)
	}
}

func TestKindFromAPIList(t *testing.T) {
	cases := map[string]Kind{
		"netrogat":           KindNetrogat,
		"custom_rkn":         KindCustom,
		"substring":          KindRKNSubstrings,
		"netrogat_substring": KindNetrogatSubstrings,
	}
	for list, want := range cases {
		got, ok := KindFromAPIList(list)
		if !ok || got != want {
			t.Errorf("KindFromAPIList(%q) = %q, %v", list, got, ok)
		}
	}
	if _, ok := KindFromAPIList("googlevideo"); ok {
		t.Fatal("googlevideo должен оставаться вне domains.Kind")
	}
}

func TestEnsureAndEmptyList(t *testing.T) {
	m, _ := newMgr(t)
	l, err := m.List(KindNetrogat)
	if err != nil || len(l) != 0 {
		t.Fatalf("List on missing file = %v, %v", l, err)
	}
	if err := m.Ensure(KindNetrogat); err != nil {
		t.Fatal(err)
	}
	path, _ := m.Path(KindNetrogat)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	// Ensure не затирает существующее содержимое.
	if _, _, err := m.Add(KindNetrogat, "a.com"); err != nil {
		t.Fatal(err)
	}
	if err := m.Ensure(KindNetrogat); err != nil {
		t.Fatal(err)
	}
	if n, _ := m.Count(KindNetrogat); n != 1 {
		t.Errorf("Count = %d, want 1", n)
	}
}

func TestAddListContains(t *testing.T) {
	m, _ := newMgr(t)
	for _, d := range []string{"b.com", "A.com", "c.org"} {
		if _, ok, err := m.Add(KindNetrogat, d); err != nil || !ok {
			t.Fatalf("Add(%s) = %v, %v", d, ok, err)
		}
	}
	norm, ok, err := m.Add(KindNetrogat, "B.COM")
	if err != nil || ok || norm != "b.com" {
		t.Fatalf("duplicate Add = %q, %v, %v", norm, ok, err)
	}
	got, err := m.List(KindNetrogat)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b.com", "a.com", "c.org"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	if ok, _ := m.Contains(KindNetrogat, "C.org"); !ok {
		t.Error("Contains(C.org) = false")
	}
	if ok, _ := m.Contains(KindNetrogat, "zzz.com"); ok {
		t.Error("Contains(zzz.com) = true")
	}
	if _, _, err := m.Add(KindNetrogat, "not valid"); !errors.Is(err, ErrInvalid) {
		t.Errorf("Add invalid err = %v", err)
	}
	if _, _, err := m.Add("bogus", "a.com"); !errors.Is(err, ErrInvalid) {
		t.Errorf("Add bogus kind err = %v", err)
	}
}

func TestListSkipsCommentsAndBlankLines(t *testing.T) {
	m, p := newMgr(t)
	if err := os.MkdirAll(p.ListsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, _ := m.Path(KindNetrogat)
	content := "# header\r\n\r\nfoo.com\r\n  bar.com  \n\n# tail\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := m.List(KindNetrogat)
	if want := []string{"foo.com", "bar.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	// Комментарии сохраняются при записи, пустые строки удаляются.
	if _, _, err := m.Add(KindNetrogat, "baz.com"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "# header") || !strings.Contains(string(raw), "# tail") {
		t.Errorf("comments lost: %q", raw)
	}
	if strings.Contains(string(raw), "\n\n") {
		t.Errorf("blank lines not removed: %q", raw)
	}
}

func TestRemove(t *testing.T) {
	m, _ := newMgr(t)
	for _, d := range []string{"a.com", "b.com", "c.com"} {
		m.Add(KindNetrogat, d)
	}
	ok, err := m.Remove(KindNetrogat, "B.com")
	if err != nil || !ok {
		t.Fatalf("Remove = %v, %v", ok, err)
	}
	ok, err = m.Remove(KindNetrogat, "b.com")
	if err != nil || ok {
		t.Fatalf("second Remove = %v, %v", ok, err)
	}
	got, _ := m.List(KindNetrogat)
	if want := []string{"a.com", "c.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	if _, err := m.Remove(KindNetrogat, "bad domain"); !errors.Is(err, ErrInvalid) {
		t.Errorf("Remove invalid err = %v", err)
	}
}

func TestRenameKeepsPosition(t *testing.T) {
	m, _ := newMgr(t)
	for _, d := range []string{"a.com", "b.com", "c.com"} {
		m.Add(KindNetrogat, d)
	}
	if err := m.Rename(KindNetrogat, "B.com", "new.com"); err != nil {
		t.Fatal(err)
	}
	got, _ := m.List(KindNetrogat)
	if want := []string{"a.com", "new.com", "c.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	if err := m.Rename(KindNetrogat, "missing.com", "x.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename missing err = %v", err)
	}
	if err := m.Rename(KindNetrogat, "a.com", "c.com"); !errors.Is(err, ErrExists) {
		t.Errorf("rename to existing err = %v", err)
	}
	// Смена регистра самого себя допустима.
	if err := m.Rename(KindNetrogat, "a.com", "A.com"); err != nil {
		t.Errorf("rename self err = %v", err)
	}
}

func TestSubstringLists(t *testing.T) {
	m, _ := newMgr(t)
	for _, k := range []Kind{KindNetrogatSubstrings, KindRKNSubstrings} {
		if _, ok, err := m.Add(k, "Google"); err != nil || !ok {
			t.Fatalf("%s Add = %v, %v", k, ok, err)
		}
		// Регистр сохраняется; дубликат без учёта регистра.
		if n, ok, err := m.Add(k, "google"); err != nil || ok || n != "google" {
			t.Errorf("%s Add dup = %q, %v, %v", k, n, ok, err)
		}
		if err := m.Rename(k, "google", "gstatic"); err != nil {
			t.Fatal(err)
		}
		got, _ := m.List(k)
		if !reflect.DeepEqual(got, []string{"gstatic"}) {
			t.Errorf("%s List = %v", k, got)
		}
		if ok, _ := m.Remove(k, "gstatic"); !ok {
			t.Errorf("%s Remove failed", k)
		}
	}
}

func TestImportClear(t *testing.T) {
	m, _ := newMgr(t)
	added, dups, skipped, err := m.Import(KindNetrogat, "a.com\nb.com,a.com\n#x\n-bad.com")
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || dups != 1 || skipped != 1 {
		t.Fatalf("Import = %d/%d/%d", added, dups, skipped)
	}
	cleared, fails, err := m.Clear(KindNetrogat)
	if err != nil || cleared != 2 || fails != 0 {
		t.Fatalf("Clear = %d, %d, %v", cleared, fails, err)
	}
}

func TestCustomStrategyAssociation(t *testing.T) {
	m, _ := newMgr(t)
	added, err := m.AddWithStrategy("Example.com", 7)
	if err != nil || !added {
		t.Fatalf("AddWithStrategy = %v, %v", added, err)
	}
	st, err := m.Strategy("example.com")
	if err != nil || st != 7 {
		t.Fatalf("Strategy = %v, %v", st, err)
	}
	// Повторное добавление обновляет стратегию.
	added, err = m.AddWithStrategy("example.com", 9)
	if err != nil || added {
		t.Fatalf("second AddWithStrategy = %v, %v", added, err)
	}
	if st, _ := m.Strategy("example.com"); st != 9 {
		t.Errorf("strategy = %v, want 9", st)
	}
	if _, err := m.AddWithStrategy("localhost", 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("single-label custom err = %v", err)
	}

	m.Add(KindCustom, "plain.org")
	entries, err := m.ListWithStrategies(KindCustom)
	if err != nil || len(entries) != 2 {
		t.Fatalf("ListWithStrategies = %v, %v", entries, err)
	}
	if entries[0].Value != "example.com" || entries[0].Strategy != 9 {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if !entries[1].Strategy.IsAuto() {
		t.Errorf("entry[1] = %+v, want auto", entries[1])
	}

	// Снятие фиксации.
	if err := m.SetStrategy("example.com", orchestra.StrategyAuto); err != nil {
		t.Fatal(err)
	}
	if st, _ := m.Strategy("example.com"); !st.IsAuto() {
		t.Errorf("strategy after clear = %v", st)
	}

	// Список без стратегий возвращает auto.
	m.Add(KindNetrogat, "n.com")
	ne, _ := m.ListWithStrategies(KindNetrogat)
	if len(ne) != 1 || !ne[0].Strategy.IsAuto() {
		t.Errorf("netrogat entries = %+v", ne)
	}
}

func TestSetStrategyRequiresMembership(t *testing.T) {
	m, _ := newMgr(t)
	if err := m.SetStrategy("nope.com", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	m.Add(KindCustom, "yes.com")
	if err := m.SetStrategy("yes.com", 3); err != nil {
		t.Fatal(err)
	}
	if st, _ := m.Strategy("yes.com"); st != 3 {
		t.Errorf("strategy = %v", st)
	}
}

func TestRemoveCustomClearsLocksEverywhere(t *testing.T) {
	m, _ := newMgr(t)
	if _, err := m.AddWithStrategy("gone.com", 5); err != nil {
		t.Fatal(err)
	}
	if err := m.Locks.SetLock("mark:2", "gone.com", "tls", 6); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Remove(KindCustom, "gone.com"); err != nil || !ok {
		t.Fatalf("Remove = %v, %v", ok, err)
	}
	for _, sc := range []string{orchestra.ScopeDefault, "mark:2"} {
		st, err := m.Locks.GetLock(sc, "gone.com", "tls")
		if err != nil || !st.IsAuto() {
			t.Errorf("scope %s lock = %v, %v; want auto", sc, st, err)
		}
	}
}

func TestRenameCustomMovesLocks(t *testing.T) {
	m, _ := newMgr(t)
	if _, err := m.AddWithStrategy("old.com", 4); err != nil {
		t.Fatal(err)
	}
	if err := m.Locks.SetLock("mark:3", "old.com", "tls", 8); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename(KindCustom, "old.com", "new.com"); err != nil {
		t.Fatal(err)
	}
	if st, _ := m.Locks.GetLock(orchestra.ScopeDefault, "new.com", "tls"); st != 4 {
		t.Errorf("default new lock = %v, want 4", st)
	}
	if st, _ := m.Locks.GetLock("mark:3", "new.com", "tls"); st != 8 {
		t.Errorf("mark:3 new lock = %v, want 8", st)
	}
	for _, sc := range []string{orchestra.ScopeDefault, "mark:3"} {
		if st, _ := m.Locks.GetLock(sc, "old.com", "tls"); !st.IsAuto() {
			t.Errorf("scope %s old lock = %v, want auto", sc, st)
		}
	}
	got, _ := m.List(KindCustom)
	if !reflect.DeepEqual(got, []string{"new.com"}) {
		t.Errorf("List = %v", got)
	}
}

func TestStrategyWithoutStore(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	if _, err := m.AddWithStrategy("a.com", 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v", err)
	}
	if err := m.SetStrategy("a.com", 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v", err)
	}
	if st, err := m.Strategy("a.com"); err != nil || !st.IsAuto() {
		t.Errorf("Strategy = %v, %v", st, err)
	}
	// Работа со списком без хранилища стратегий допустима.
	if _, ok, err := m.Add(KindCustom, "a.com"); err != nil || !ok {
		t.Errorf("Add = %v, %v", ok, err)
	}
	if ok, err := m.Remove(KindCustom, "a.com"); err != nil || !ok {
		t.Errorf("Remove = %v, %v", ok, err)
	}
	if n, err := m.RestoreFromLocks(); err != nil || n != 0 {
		t.Errorf("RestoreFromLocks = %d, %v", n, err)
	}
}

func TestRestoreFromLocks(t *testing.T) {
	m, _ := newMgr(t)
	if err := m.Locks.SetLock(orchestra.ScopeDefault, "lost.com", "tls", 2); err != nil {
		t.Fatal(err)
	}
	if err := m.Locks.SetLock(orchestra.ScopeDefault, "1", "tls", 3); err != nil { // профиль, не домен
		t.Fatal(err)
	}
	m.Add(KindCustom, "kept.com")
	n, err := m.RestoreFromLocks()
	if err != nil || n != 1 {
		t.Fatalf("RestoreFromLocks = %d, %v; want 1", n, err)
	}
	got, _ := m.List(KindCustom)
	if want := []string{"kept.com", "lost.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	if n, _ := m.RestoreFromLocks(); n != 0 {
		t.Errorf("second RestoreFromLocks = %d, want 0", n)
	}
}

func TestConcurrentAdds(t *testing.T) {
	m, _ := newMgr(t)
	done := make(chan struct{})
	const n = 20
	for i := 0; i < n; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			m.Add(KindNetrogat, "h"+string(rune('a'+i))+".com")
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if c, _ := m.Count(KindNetrogat); c != n {
		t.Errorf("Count = %d, want %d", c, n)
	}
}
