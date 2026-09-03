package sysmon

// Snapshot — загрузка хоста для UI (Linux: /proc, иначе частично недоступно).
type Snapshot struct {
	CPUPct      *float64 `json:"cpu_pct,omitempty"`
	MemUsedPct  *float64 `json:"mem_used_pct,omitempty"`
	DiskFreePct *float64 `json:"disk_free_pct,omitempty"`
	DiskFreeGB  *float64 `json:"disk_free_gb,omitempty"`
}
