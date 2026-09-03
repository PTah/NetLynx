package swcfg

import "testing"

func TestUbiquitiPoEOpmodeCLI(t *testing.T) {
	cases := map[string]string{
		"off":  "poe opmode shutdown",
		"24v":  "poe opmode passive24v",
		"poe+": "poe opmode auto",
		"auto": "poe opmode auto",
	}
	for in, want := range cases {
		got, err := UbiquitiPoEOpmodeCLI(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", in, got, want)
		}
	}
	if _, err := UbiquitiPoEOpmodeCLI("weird"); err == nil {
		t.Fatal("expected error")
	}
}
