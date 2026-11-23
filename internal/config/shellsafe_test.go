package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSourceSafeOK(t *testing.T) {
	ok := []string{
		"FWTYPE=iptables\n",
		"NFQWS2_OPT=\"\n--new\n--qnum=200\n\"\nFWTYPE=nftables\n",
		"IPSET_OPT=\"hashsize 262144 maxelem $SET_MAXELEM\"\n",
		"# comment\n\nWS_USER=nobody\n",
		"CLIENT_SCOPE_MARK_MASK=\n",
		"A=\"one line\"\n",
	}
	for _, s := range ok {
		if err := ValidateSourceSafe(s); err != nil {
			t.Fatalf("ok rejected: %v\n%s", err, s)
		}
	}
}

func TestValidateSourceSafeRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"bare command", "reboot\nFWTYPE=iptables\n"},
		{"cmd after assign", "FWTYPE=iptables; reboot\n"},
		{"pipe", "FWTYPE=iptables | reboot\n"},
		{"dollar paren", "X=$(reboot)\n"},
		{"backtick", "X=`reboot`\n"},
		{"multiline breakout", "FOO=\"\n\"; reboot; echo \"\n"},
		{"breakout same line", "FOO=\"x\"; reboot\n"},
		{"subshell in quoted", "NFQWS2_OPT=\"\n$(reboot)\n\"\n"},
		{"unclosed", "NFQWS2_OPT=\"\n--new\n"},
		{"nul", "A=\x00\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSourceSafe(tc.in); err == nil {
				t.Fatalf("expected reject:\n%s", tc.in)
			}
		})
	}
}

func TestValidateSourceSafeDefaultAsset(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "config.default")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("asset: %v", err)
	}
	if err := ValidateSourceSafe(string(data)); err != nil {
		t.Fatalf("config.default: %v", err)
	}
}

func FuzzValidateSourceSafe(f *testing.F) {
	for _, s := range []string{
		"FWTYPE=iptables\n",
		"NFQWS2_OPT=\"\n--new\n--qnum=200\n\"\n",
		"X=$(reboot)\n",
		"FOO=\"\n\"; reboot; echo \"\n",
		"A=\"one line\"\n",
		"# comment\n",
		"",
		"reboot\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateSourceSafe(s)
		if err != nil {
			return
		}
		// Принятый конфиг не должен содержать опасные конструкции.
		if strings.Contains(s, "$(") || strings.Contains(s, "`") || strings.ContainsRune(s, 0) {
			t.Fatalf("accepted unsafe input: %q", s)
		}
	})
}
