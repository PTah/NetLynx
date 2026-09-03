package store

import "testing"

func TestPortIgnoreFromModeSoftAndAll(t *testing.T) {
	soft := PortIgnoreFromMode(1, 24, IgnoreModeSoft)
	if soft == nil || soft.BlockEvents || !soft.BlockNotify || !soft.BlockActions {
		t.Fatal("soft preset")
	}
	if soft.EventTypes == nil || *soft.EventTypes != IgnoreSoftEventTypes {
		t.Fatal("soft event_types")
	}
	all := PortIgnoreFromMode(1, 24, IgnoreModeAll)
	if all == nil || !all.BlockEvents || !all.BlockNotify || !all.BlockActions {
		t.Fatal("all preset")
	}
	if all.EventTypes != nil {
		t.Fatal("all should have nil event_types")
	}
}

func TestClassifyPortIgnoreMode(t *testing.T) {
	all := PortIgnoreFromMode(1, 1, IgnoreModeAll)
	if ClassifyPortIgnoreMode(*all) != IgnoreModeAll {
		t.Fatal("all classify")
	}
	soft := PortIgnoreFromMode(1, 1, IgnoreModeSoft)
	if ClassifyPortIgnoreMode(*soft) != IgnoreModeSoft {
		t.Fatal("soft classify")
	}
	bad := PortIgnoreFromMode(1, 1, IgnoreModeAll)
	bad.BlockEvents = false
	if ClassifyPortIgnoreMode(*bad) == IgnoreModeAll {
		t.Fatal("all comment without block_events must not be all")
	}
}
