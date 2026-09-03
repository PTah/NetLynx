package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleDeviceMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	metricType := r.URL.Query().Get("metric_type")
	if metricType == "" {
		metricType = "cpu_pct"
	}
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	// all_ports=1 — все if_index для metric_type (для спарклайнов util_*).
	if r.URL.Query().Get("all_ports") == "1" || strings.EqualFold(r.URL.Query().Get("all_ports"), "true") {
		samples, err := s.st.ListPortMetricSamples(r.Context(), id, []string{metricType}, from, to)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if samples == nil {
			samples = []store.MetricSample{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"device_id":   id,
			"metric_type": metricType,
			"samples":     samples,
		})
		return
	}
	var ifIndex *int
	if v := r.URL.Query().Get("if_index"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "неверный if_index")
			return
		}
		ifIndex = &n
	}
	samples, err := s.st.ListMetricSamples(r.Context(), id, metricType, ifIndex, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if samples == nil {
		samples = []store.MetricSample{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":   id,
		"metric_type": metricType,
		"samples":     samples,
	})
}

func (s *Server) handleDeviceTrafficSeries(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	minutes := 60
	if v := r.URL.Query().Get("minutes"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 5 || n > 7*24*60 {
			writeError(w, http.StatusBadRequest, "неверный minutes")
			return
		}
		minutes = n
	}
	to := time.Now()
	from := to.Add(-time.Duration(minutes) * time.Minute)
	samples, err := s.st.ListPortMetricSamples(r.Context(), id, []string{"rate_in_bps", "rate_out_bps"}, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type pt struct {
		T time.Time `json:"t"`
		V float32   `json:"v"`
	}
	type portSeries struct {
		Rx []pt `json:"rx"`
		Tx []pt `json:"tx"`
	}
	byIf := make(map[int]*portSeries)
	for _, sm := range samples {
		if sm.IfIndex == nil {
			continue
		}
		idx := *sm.IfIndex
		ps := byIf[idx]
		if ps == nil {
			ps = &portSeries{Rx: []pt{}, Tx: []pt{}}
			byIf[idx] = ps
		}
		p := pt{T: sm.SampledAt, V: sm.Value}
		switch sm.MetricType {
		case "rate_in_bps":
			ps.Rx = append(ps.Rx, p)
		case "rate_out_bps":
			ps.Tx = append(ps.Tx, p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id": id,
		"from":      from,
		"to":        to,
		"ports":     byIf,
	})
}
