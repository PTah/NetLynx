package store

// DBPoolStats — снимок пула PostgreSQL для мониторинга.
type DBPoolStats struct {
	TotalConns    int32 `json:"total_conns"`
	AcquiredConns int32 `json:"acquired_conns"`
	IdleConns     int32 `json:"idle_conns"`
	AcquireTotal  int64 `json:"acquire_total"`
	MaxConns      int32 `json:"max_conns"`
}

func (s *Store) DBPoolStats() DBPoolStats {
	if s.pool == nil {
		return DBPoolStats{}
	}
	st := s.pool.Stat()
	return DBPoolStats{
		TotalConns:    st.TotalConns(),
		AcquiredConns: st.AcquiredConns(),
		IdleConns:     st.IdleConns(),
		AcquireTotal:  st.AcquireCount(),
		MaxConns:      st.MaxConns(),
	}
}
