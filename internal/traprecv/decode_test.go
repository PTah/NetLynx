package traprecv

import (
	"strings"
	"testing"
)

func TestDecodeTrapEdgeSwitchLogin(t *testing.T) {
	t.Parallel()
	d := DecodeTrap(".1.3.6.1.4.1.4413.1.1.1.0.29", nil, map[string]interface{}{
		"varbinds": []interface{}{
			map[string]interface{}{"oid": ".1.3.6.1.2.1.1.3.0", "value": "2000697555"},
		},
	})
	if d.Label != "loginSessionStartStopTrap" {
		t.Fatalf("label=%q", d.Label)
	}
	if !strings.Contains(d.Summary, "CLI-сессия") {
		t.Fatalf("summary=%q", d.Summary)
	}
}

func TestDecodeTrapLinkDown(t *testing.T) {
	t.Parallel()
	idx := 9
	d := DecodeTrap("1.3.6.1.6.3.1.1.5.3", &idx, map[string]interface{}{
		"varbinds": []interface{}{
			map[string]interface{}{"oid": "1.3.6.1.2.1.2.2.1.1.9", "value": "9"},
			map[string]interface{}{"oid": "1.3.6.1.2.1.2.2.1.8.9", "value": "2"},
		},
	})
	if d.Label != "linkDown" {
		t.Fatalf("label=%q", d.Label)
	}
	if !strings.Contains(d.Summary, "ifIndex 9") || !strings.Contains(d.Summary, "oper=down") {
		t.Fatalf("summary=%q", d.Summary)
	}
}
