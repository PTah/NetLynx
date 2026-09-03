package poller

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestSnapshotBitsPerSecond(t *testing.T) {
	t.Parallel()
	hs := int64(1000)
	sp := int64(100_000_000)
	cases := []struct {
		name string
		old  store.InterfaceSnapshot
		want int64
	}{
		{name: "high speed", old: store.InterfaceSnapshot{IfHighSpeed: &hs}, want: 1_000_000_000},
		{name: "ifSpeed only", old: store.InterfaceSnapshot{IfSpeed: &sp}, want: 100_000_000},
		{name: "empty", old: store.InterfaceSnapshot{}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotBitsPerSecond(tc.old); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}
