package swcfg

// UbiquitiFlowControlCLI — EdgeSwitch Fastpath.
func UbiquitiFlowControlCLI(on bool) string {
	if on {
		return "flowcontrol"
	}
	return "no flowcontrol"
}
