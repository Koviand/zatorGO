package sweep

import (
	"reflect"
	"testing"
)

func TestNormalizeWorkers(t *testing.T) {
	if got := NormalizeWorkers(nil); !reflect.DeepEqual(got, Phases) {
		t.Fatalf("nil = %v", got)
	}
	if got := NormalizeWorkers([]string{}); !reflect.DeepEqual(got, Phases) {
		t.Fatalf("empty = %v", got)
	}
	got := NormalizeWorkers([]string{"ds", "yt", "nope"})
	want := []string{WorkerYT, WorkerDS}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// мусор / опечатка — не полный прогон
	if got := NormalizeWorkers([]string{"broken", "nope"}); got != nil {
		t.Fatalf("garbage = %v want nil", got)
	}
	if got := NormalizeWorkers([]string{"", "  "}); got != nil {
		t.Fatalf("blanks = %v want nil", got)
	}
}

func TestProfileWorker(t *testing.T) {
	w, ok := ProfileWorker(4)
	if !ok || w != WorkerDS {
		t.Fatalf("profile 4 = %q %v", w, ok)
	}
	if _, ok := ProfileWorker(9); ok {
		t.Fatal("profile 9 must be false")
	}
}
