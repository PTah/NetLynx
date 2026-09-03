package swcfg

import (
	"io"
	"strings"
	"testing"
)

func TestPortConfigBody(t *testing.T) {
	desc := "TEMP VIDEO"
	up := true
	steps, err := portConfigBody(VendorUbiquiti, "0/1", PortChange{
		Interface:   "0/1",
		Description: &desc,
		AdminUp:     &up,
		Write:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(steps, "\n")
	for _, want := range []string{"interface 0/1", "description 'TEMP VIDEO'", "no shutdown", "exit", "write memory"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, steps)
		}
	}
}

func TestQuoteCLIDescription(t *testing.T) {
	if quoteCLIDescription("uplink") != "uplink" {
		t.Fatal("single token unchanged")
	}
	if quoteCLIDescription("ROOM 29.14") != "'ROOM 29.14'" {
		t.Fatalf("got %q", quoteCLIDescription("ROOM 29.14"))
	}
	if quoteCLIDescription(`it's ok`) != `"it's ok"` {
		t.Fatalf("got %q", quoteCLIDescription(`it's ok`))
	}
}

func TestPortConfigBodyEltexIsolate(t *testing.T) {
	on := true
	steps, err := portConfigBody(VendorEltex, "GigabitEthernet 1/0/5", PortChange{Isolate: &on, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(steps, "\n")
	if !strings.Contains(joined, "switchport protected-port") {
		t.Fatalf("%v", steps)
	}
}

func TestPortConfigBodySNRIsolate(t *testing.T) {
	on := true
	steps, err := portConfigBody(VendorSNR, "Ethernet 1/0/5", PortChange{Isolate: &on, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(steps, "\n")
	if !strings.Contains(joined, "isolate-port group netlynx") {
		t.Fatalf("%v", steps)
	}
}

func TestInterpretPortCLIIgnoresBannerInvalid(t *testing.T) {
	out := "Welcome\nInvalid input somewhere in motd\n(UBNT) (Config)#\ninterface 0/3\n(Interface 0/3)#\ndescription ok\n"
	if err := interpretPortCLI(out); err != nil {
		t.Fatal(err)
	}
}

func TestInterpretPortCLICatchesConfigError(t *testing.T) {
	out := "(UBNT) (Config)#\ninterface 0/3\nInvalid input detected at '^' marker.\n"
	if err := interpretPortCLI(out); err == nil {
		t.Fatal("expected error")
	}
}

func TestIsSSHSessionEOF(t *testing.T) {
	if !isSSHSessionEOF(io.EOF) {
		t.Fatal("io.EOF")
	}
}

func TestInterpretPortCLIDescriptionEcho(t *testing.T) {
	out := "(UBNT) #\nen\nconfigure\n(UBNT) (Config)#\ninterface 0/3\n(Interface 0/3)#\ndescription Test Description for NetLynx\n"
	if err := interpretPortCLI(out); err != nil {
		t.Fatal(err)
	}
}
