package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"faulthub/internal/store"
)

func chartLabels(counts []store.Count) string {
	labels := make([]string, len(counts))
	for i, c := range counts {
		labels[i] = shortDay(c.Label)
	}
	out, _ := json.Marshal(labels)
	return string(out)
}

func chartValues(counts []store.Count) string {
	values := make([]int64, len(counts))
	for i, c := range counts {
		values[i] = c.Count
	}
	out, _ := json.Marshal(values)
	return string(out)
}

func shortDay(label string) string {
	if t, err := time.Parse("2006-01-02", label); err == nil {
		return t.Format("Jan 2")
	}
	return label
}

type dashboardData struct {
	baseData
	Trend     []store.Count
	Overview  []store.AppOverview
	TopIssues []store.Issue
}

func (s *Server) pageDashboard(w http.ResponseWriter, r *http.Request) {
	counts, err := s.Store.ReportsPerDay(r.Context(), 0, 30)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	overview, err := s.Store.AppOverview(r.Context())
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	top, err := s.Store.TopIssues(r.Context(), 0, 10)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, "dashboard.html", dashboardData{
		baseData:  baseData{Title: "Overview", Authed: true, CSRF: s.csrfToken(r)},
		Trend:     counts,
		Overview:  overview,
		TopIssues: top,
	})
}

func (s *Server) appID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}
