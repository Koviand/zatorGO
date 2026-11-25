package googlevideo

import "testing"

func TestSNIHostKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"rr1---sn-2oi5h5-54.googlevideo.com", "sn-2oi5h5-54", true},
		{"rr18---sn-pmcg-bg0s.googlevideo.com", "sn-pmcg-bg0s", true},
		{"rr5---sn-2oi5h5-5j.googlevideo.com.", "sn-2oi5h5-5j", true},
		{"RR2---SN-Q4FL6ND6.googlevideo.com", "sn-q4fl6nd6", true},
		{"sn-u1i5h5-5q.googlevideo.com", "sn-u1i5h5-5q", true},
		{"rr1---sn-bad_id.googlevideo.com", "", false}, // underscore вне [a-z0-9-]
		{"sn-ntqe6nes", "sn-ntqe6nes", true},
		{"SN-Q4FL6ND6", "sn-q4fl6nd6", true},
		{"redirector.googlevideo.com", "", false},
		{"www.youtube.com", "", false},
		{"", "", false},
	}
	if !IsRRMediaHost("rr18---sn-pmcg-bg0s.googlevideo.com") {
		t.Fatal("IsRRMediaHost expected true")
	}
	if IsRRMediaHost("redirector.googlevideo.com") {
		t.Fatal("IsRRMediaHost expected false for redirector")
	}
	for _, tc := range cases {
		got, ok := SNIHostKey(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("SNIHostKey(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
