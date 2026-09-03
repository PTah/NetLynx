//go:build linux

package sysmon

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func ReadSnapshot() Snapshot {
	var out Snapshot
	if pct := readCPUPct(); pct != nil {
		out.CPUPct = pct
	}
	if pct := readMemUsedPct(); pct != nil {
		out.MemUsedPct = pct
	}
	if freePct, freeGB := readDiskFree("/"); freePct != nil {
		out.DiskFreePct = freePct
		out.DiskFreeGB = freeGB
	}
	return out
}

func readCPUPct() *float64 {
	a1, err := readProcStatCPU()
	if err != nil {
		return nil
	}
	time.Sleep(120 * time.Millisecond)
	a2, err := readProcStatCPU()
	if err != nil {
		return nil
	}
	idle := a2.idle - a1.idle
	total := a2.total - a1.total
	if total <= 0 {
		return nil
	}
	used := float64(total-idle) / float64(total) * 100
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}
	return &used
}

type cpuAgg struct {
	idle  uint64
	total uint64
}

func readProcStatCPU() (cpuAgg, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuAgg{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return cpuAgg{}, sc.Err()
	}
	line := sc.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return cpuAgg{}, os.ErrInvalid
	}
	fields := strings.Fields(line)[1:]
	if len(fields) < 4 {
		return cpuAgg{}, os.ErrInvalid
	}
	vals := make([]uint64, 0, len(fields))
	for _, fld := range fields {
		n, err := strconv.ParseUint(fld, 10, 64)
		if err != nil {
			return cpuAgg{}, err
		}
		vals = append(vals, n)
	}
	var total uint64
	for _, v := range vals {
		total += v
	}
	return cpuAgg{idle: vals[3], total: total}, nil
}

func readMemUsedPct() *float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer f.Close()
	var total, available float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = parseMeminfoKB(line)
		}
	}
	if total <= 0 {
		return nil
	}
	usedPct := (total - available) / total * 100
	if usedPct < 0 {
		usedPct = 0
	}
	if usedPct > 100 {
		usedPct = 100
	}
	return &usedPct
}

func parseMeminfoKB(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.ParseFloat(fields[1], 64)
	return n
}

func readDiskFree(path string) (*float64, *float64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return nil, nil
	}
	total := float64(st.Blocks) * float64(st.Bsize)
	free := float64(st.Bavail) * float64(st.Bsize)
	if total <= 0 {
		return nil, nil
	}
	freePct := free / total * 100
	freeGB := free / (1024 * 1024 * 1024)
	return &freePct, &freeGB
}
