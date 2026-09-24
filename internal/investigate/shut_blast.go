package investigate

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ShutDownstreamHit — свитч ниже по топологии от соседа за портом.
type ShutDownstreamHit struct {
	DeviceID   int64   `json:"device_id"`
	DeviceName string  `json:"device_name"`
	DeviceHost string  `json:"device_host,omitempty"`
	Hop        int     `json:"hop"`
	PathIDs    []int64 `json:"path_device_ids,omitempty"`
	Redundant  bool    `json:"redundant,omitempty"`
}

// ShutCutPath — путь neighbor→root и alternate в обход Y.
type ShutCutPath struct {
	NeighborDeviceID   int64   `json:"neighbor_device_id"`
	NeighborDeviceName string  `json:"neighbor_device_name,omitempty"`
	PathViaY           []int64 `json:"path_via_y,omitempty"`
	AlternatePath      []int64 `json:"alternate_path,omitempty"`
	Redundant          bool    `json:"redundant"`
}

// ShutBlastResult — multi-hop дополнение к локальному shut-impact.
type ShutBlastResult struct {
	Downstream     []ShutDownstreamHit `json:"downstream"`
	CutPaths       []ShutCutPath       `json:"cut_paths"`
	DownstreamCount int                `json:"downstream_count"`
	TopologySource string              `json:"topology_source,omitempty"`
	RootDeviceID   int64               `json:"root_device_id,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}

// BuildShutBlast — для каждого resolved neighbor за портом: walk вниз + cut paths.
func (b *Builder) BuildShutBlast(ctx context.Context, deviceID int64, neighborIDs []int64, maxDepth int) (*ShutBlastResult, error) {
	out := &ShutBlastResult{
		Downstream: []ShutDownstreamHit{},
		CutPaths:   []ShutCutPath{},
	}
	g, ok, err := LoadBlastGraph(ctx, b.St)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, nil
	}
	out.TopologySource = g.Source
	out.RootDeviceID = g.RootID
	maxDepth = EffectiveVLANBlastMaxDepth(maxDepth)
	if maxDepth < 1 {
		maxDepth = DefaultVLANBlastMaxDepth
	}

	names := b.DeviceNames(ctx)
	hosts := map[int64]string{}
	if devs, err := b.St.ListDevices(ctx); err == nil {
		for _, d := range devs {
			hosts[d.ID] = d.Host
		}
	}

	seenDown := map[int64]struct{}{}
	always := func(int64) bool { return true }

	for _, nid := range neighborIDs {
		if nid <= 0 || nid == deviceID {
			continue
		}
		nm := names[nid]
		if nm == "" {
			nm = fmt.Sprintf("#%d", nid)
		}
		pathVia := ShortestPath(g.Adj, nid, g.RootID, 0)
		alt := ShortestPath(g.Adj, nid, g.RootID, deviceID)
		redundant := len(alt) > 0 && (len(pathVia) == 0 || !samePath(pathVia, alt))
		if g.RootID > 0 {
			out.CutPaths = append(out.CutPaths, ShutCutPath{
				NeighborDeviceID:   nid,
				NeighborDeviceName: nm,
				PathViaY:           pathVia,
				AlternatePath:      alt,
				Redundant:          redundant,
			})
		}

		desc := WalkDownWithVLAN(g.Adj, nid, deviceID, maxDepth, g.Dist, always)
		for _, d := range desc {
			if _, ok := seenDown[d.ID]; ok {
				continue
			}
			seenDown[d.ID] = struct{}{}
			path := ShortestPath(g.Adj, nid, d.ID, deviceID)
			if len(path) == 0 {
				path = ShortestPath(g.Adj, nid, d.ID, 0)
			}
			dnm := names[d.ID]
			if dnm == "" {
				dnm = fmt.Sprintf("#%d", d.ID)
			}
			red := false
			if g.RootID > 0 {
				altN := ShortestPath(g.Adj, d.ID, g.RootID, deviceID)
				red = len(altN) > 0
			}
			out.Downstream = append(out.Downstream, ShutDownstreamHit{
				DeviceID:   d.ID,
				DeviceName: dnm,
				DeviceHost: hosts[d.ID],
				Hop:        d.Depth,
				PathIDs:    path,
				Redundant:  red,
			})
		}
	}

	out.DownstreamCount = len(out.Downstream)
	if out.DownstreamCount > 0 {
		names := make([]string, 0, len(out.Downstream))
		for _, d := range out.Downstream {
			nm := strings.TrimSpace(d.DeviceName)
			if nm == "" {
				nm = fmt.Sprintf("#%d", d.DeviceID)
			}
			names = append(names, nm)
		}
		sort.Strings(names)
		out.Warnings = append(out.Warnings,
			fmt.Sprintf("Отключив этот порт, вы можете отрезать от сети свичи: %s.", formatNamedList(names, 16)))
	}
	for _, cp := range out.CutPaths {
		if !cp.Redundant && len(cp.PathViaY) > 0 {
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("Сосед %s: нет известного пути к root в обход этого узла — риск отрезать ветку.", cp.NeighborDeviceName))
			break
		}
	}
	return out, nil
}

func samePath(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
