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

func TestUbiquitiPoEResetCLI(t *testing.T) {
	got, err := UbiquitiPoEResetCLI(10)
	if err != nil {
		t.Fatal(err)
	}
	if got != "poe reset 10" {
		t.Fatalf("got %q", got)
	}
	if _, err := UbiquitiPoEResetCLI(0); err == nil {
		t.Fatal("expected error")
	}
	if _, err := UbiquitiPoEResetCLI(61); err == nil {
		t.Fatal("expected error")
	}
}

func TestCompactUbiquitiInterfaceRange(t *testing.T) {
	got, ok := CompactUbiquitiInterfaceRange([]string{"0/7", "0/4", "0/5", "0/6"})
	if !ok || got != "0/4-0/7" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := CompactUbiquitiInterfaceRange([]string{"0/4", "0/6"}); ok {
		t.Fatal("gap should fail")
	}
	if _, ok := CompactUbiquitiInterfaceRange([]string{"GigabitEthernet1/0/1", "GigabitEthernet1/0/2"}); ok {
		t.Fatal("non-ubnt names")
	}
}
