package web

import (
	"html/template"
	"net/http"
	"strconv"
	"time"

	"faulthub/internal/charts"
	"faulthub/internal/store"
)

func toPoints(counts []store.Count) []charts.Point {
	pts := make([]charts.Point, len(counts))
	for i, c := range counts {
		pts[i] = charts.Point{Label: shortDay(c.Label), Value: c.Count}
	}
	return pts
}

func shortDay(label string) string {
	if t, err := time.Parse("2006-01-02", label); err == nil {
		return t.Format("Jan 2")
	}
	return label
}

func toBars(counts []store.Count) []charts.Bar {
	bars := make([]charts.Bar, len(counts))
	for i, c := range counts {
		bars[i] = charts.Bar{Label: c.Label, Value: c.Count}
	}
	return bars
}

func lineChart(counts []store.Count, w, h int) template.HTML {
	return charts.Line(toPoints(counts), w, h)
}

type dashboardData struct {
	baseData
	Chart     template.HTML
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
		Chart:     lineChart(counts, 900, 240),
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
