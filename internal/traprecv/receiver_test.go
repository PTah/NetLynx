package traprecv

import "testing"

func TestCommunityAllowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		expected string
		got      string
		want     bool
	}{
		{name: "no filter", expected: "", got: "public", want: true},
		{name: "no filter empty packet", expected: "", got: "", want: true},
		{name: "match", expected: "secret", got: "secret", want: true},
		{name: "mismatch", expected: "secret", got: "public", want: false},
		{name: "empty packet must not bypass", expected: "secret", got: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := communityAllowed(tc.expected, tc.got); got != tc.want {
				t.Fatalf("communityAllowed(%q, %q)=%v want %v", tc.expected, tc.got, got, tc.want)
			}
		})
	}
}
