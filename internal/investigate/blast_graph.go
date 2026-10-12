package investigate

import (
	"context"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// BlastGraph — единый скелет из topology blast cache (lldp+cdp+manual).
type BlastGraph struct {
	RootID     int64
	RootSource string
	Adj        map[int64][]int64
	Dist       map[int64]int
	Edges      []store.TopologyBlastEdge
	Cache      *store.TopologyBlastCache
	Source     string // blast_cache | empty
}

// LoadBlastGraph читает кэш; ok=false если пусто/не собран.
func LoadBlastGraph(ctx context.Context, st *store.Store) (*BlastGraph, bool, error) {
	if st == nil {
		return nil, false, nil
	}
	cache, ok, err := st.LoadTopologyBlastCache(ctx)
	if err != nil {
		return nil, false, err
	}
	if !ok || cache == nil || len(cache.Adj) == 0 {
		return nil, false, nil
	}
	root := cache.RootDeviceID
	return &BlastGraph{
		RootID:     root,
		RootSource: cache.RootSource,
		Adj:        cache.Adj,
		Dist:       cache.Dist,
		Edges:      cache.Edges,
		Cache:      cache,
		Source:     "blast_cache",
	}, true, nil
}

// DeviceNames — id→имя для path/UI.
func (b *Builder) DeviceNames(ctx context.Context) map[int64]string {
	names := map[int64]string{}
	if b == nil || b.St == nil {
		return names
	}
	devs, err := b.St.ListDevices(ctx)
	if err != nil {
		return names
	}
	for _, d := range devs {
		n := d.Name
		if n == "" {
			n = d.Host
		}
		names[d.ID] = n
	}
	return names
}
