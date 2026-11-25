package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteIfChangedSkipsSameContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	payload := []byte("hello\n")
	if err := WriteIfChanged(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	st1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteIfChanged(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	st2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("identical content must not rewrite file")
	}
	if err := WriteIfChanged(path, []byte("bye\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "bye\n" {
		t.Fatalf("got %q", got)
	}
}
