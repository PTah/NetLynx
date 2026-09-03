package swcfg

// UbiquitiIsolateCLI — EdgeSwitch Fastpath (не XP): UISP «Isolate port».
// Группа 1 — как в типичных примерах Ubiquiti; порты в одной группе не видят друг друга.
func UbiquitiIsolateCLI(isolated bool) string {
	if isolated {
		return "switchport protected 1"
	}
	return "no switchport protected"
}
