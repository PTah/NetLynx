package swcfg

import "testing"

func TestInterpretWriteMemoryCLI(t *testing.T) {
	if err := interpretWriteMemoryCLI("SNR# write memory\n[OK]\nSNR#"); err != nil {
		t.Fatal(err)
	}
	if err := interpretWriteMemoryCLI("Building configuration...\n[OK]\nSwitch#"); err != nil {
		t.Fatal(err)
	}
	if err := interpretWriteMemoryCLI("% Invalid input detected at '^' marker."); err == nil {
		t.Fatal("expected error")
	}
}
