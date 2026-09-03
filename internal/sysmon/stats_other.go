//go:build !linux

package sysmon

func ReadSnapshot() Snapshot {
	return Snapshot{}
}
