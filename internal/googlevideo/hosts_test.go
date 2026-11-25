package googlevideo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHostInput(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"rr1---sn-2oi5h5-54.googlevideo.com", "rr1---sn-2oi5h5-54.googlevideo.com", true},
		{"https://rr1---sn-2oi5h5-54.googlevideo.com", "rr1---sn-2oi5h5-54.googlevideo.com", true},
		{"https://rr1---sn-2oi5h5-54.googlevideo.com/videoplayback?x=1", "rr1---sn-2oi5h5-54.googlevideo.com", true},
		{"HTTPS://RR1---SN-ABC.googlevideo.com:443/path", "rr1---sn-abc.googlevideo.com", true},
		{"googlevideo.com", "", false},
		{"example.com", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseHostInput(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseHostInput(%q)=(%q,%v) want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestHostsFileAddDedupMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, HostsFileName)
	h := NewHostsFile(path)

	added, err := h.Add("https://rr1---sn-aaa.googlevideo.com/x")
	if err != nil || !added {
		t.Fatalf("add1: %v %v", added, err)
	}
	added, err = h.Add("rr1---sn-aaa.googlevideo.com")
	if err != nil || added {
		t.Fatalf("dup should not add: %v %v", added, err)
	}
	n, err := h.MergeFrom([]string{
		"rr2---sn-bbb.googlevideo.com",
		"https://rr1---sn-aaa.googlevideo.com",
		"bad.com",
	})
	if err != nil || n != 1 {
		t.Fatalf("merge: n=%d err=%v", n, err)
	}
	list, err := h.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := h.Remove("rr1---sn-aaa.googlevideo.com"); err != nil {
		t.Fatal(err)
	}
	list, _ = h.List()
	if len(list) != 1 || list[0] != "rr2---sn-bbb.googlevideo.com" {
		t.Fatalf("after remove: %v", list)
	}
	if err := h.Clear(); err != nil {
		t.Fatal(err)
	}
	list, _ = h.List()
	if len(list) != 0 {
		t.Fatalf("clear: %v", list)
	}
}

func TestHostsFileMergeSNILog(t *testing.T) {
	dir := t.TempDir()
	h := NewHostsFile(filepath.Join(dir, HostsFileName))
	logPath := filepath.Join(dir, SNILogFileName)
	content := "rr3---sn-ccc.googlevideo.com\t1710000000\n" +
		"rr3---sn-ccc.googlevideo.com\t1710000001\n" +
		"not-gv.example.com\t1\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := h.MergeSNILog(logPath)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	list, _ := h.List()
	if len(list) != 1 || list[0] != "rr3---sn-ccc.googlevideo.com" {
		t.Fatalf("list=%v", list)
	}
}

func TestHostsFileMergeAutoLocks(t *testing.T) {
	dir := t.TempDir()
	h := NewHostsFile(filepath.Join(dir, HostsFileName))
	n, err := h.MergeAutoLocks([]AutoLock{
		{Askey: "2", Host: "sn-ntqe6nes", Strategy: 41},
		{Askey: "2", Host: "rr2---sn-aaa.googlevideo.com", Strategy: 41},
		{Askey: "2", Host: "not-a-key", Strategy: 1},
	})
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	list, _ := h.List()
	want := map[string]bool{
		"rr1---sn-ntqe6nes.googlevideo.com": true,
		"rr2---sn-aaa.googlevideo.com":      true,
	}
	if len(list) != 2 {
		t.Fatalf("list=%v", list)
	}
	for _, got := range list {
		if !want[got] {
			t.Fatalf("unexpected %q in %v", got, list)
		}
	}
}

func TestSyntheticRRHost(t *testing.T) {
	got, ok := SyntheticRRHost("sn-abc")
	if !ok || got != "rr1---sn-abc.googlevideo.com" {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
	got, ok = SyntheticRRHost("rr5---sn-xyz.googlevideo.com")
	if !ok || got != "rr5---sn-xyz.googlevideo.com" {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
}

func TestObserveWritesHostsFile(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, HostsFileName)
	c := NewCollector(CollectorConfig{Dir: dir, HostsFile: hostsPath})
	_, err := c.Observe(Observation{
		Host: "rr4---sn-ddd.googlevideo.com",
		IP:   "8.8.8.8",
		TTL:  60,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := NewHostsFile(hostsPath).List()
	if err != nil || len(list) != 1 || list[0] != "rr4---sn-ddd.googlevideo.com" {
		t.Fatalf("hosts=%v err=%v", list, err)
	}
}
