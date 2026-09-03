package web

import (
	"html/template"
	"net/http"
	"strings"

	"faulthub/internal/keygen"
	"faulthub/internal/store"
)

type appsData struct {
	baseData
	Overview   []store.AppOverview
	NewAppName string
	NewKey     string
	Error      string
}

func (s *Server) pageApps(w http.ResponseWriter, r *http.Request) {
	overview, err := s.Store.AppOverview(r.Context())
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, "apps.html", appsData{
		baseData: baseData{Title: "Apps", Authed: true, CSRF: s.csrfToken(r)},
		Overview: overview,
	})
}

func (s *Server) submitCreateApp(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" || len(name) > 64 {
		http.Error(w, "app name required (max 64 chars)", http.StatusBadRequest)
		return
	}
	key, hash, err := keygen.NewAPIKey()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if _, err := s.Store.CreateApp(r.Context(), name, hash); err != nil {
		s.renderAppsError(w, r, "Could not create app (name already used?).")
		return
	}
	overview, err := s.Store.AppOverview(r.Context())
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, "apps.html", appsData{
		baseData:   baseData{Title: "Apps", Authed: true, CSRF: s.csrfToken(r)},
		Overview:   overview,
		NewAppName: name,
		NewKey:     key,
	})
}

func (s *Server) renderAppsError(w http.ResponseWriter, r *http.Request, msg string) {
	overview, err := s.Store.AppOverview(r.Context())
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusBadRequest, "apps.html", appsData{
		baseData: baseData{Title: "Apps", Authed: true, CSRF: s.csrfToken(r)},
		Overview: overview,
		Error:    msg,
	})
}

type appDetailData struct {
	baseData
	App       store.App
	Stats     store.AppStats
	Chart     template.HTML
	ByVersion []store.Count
	ByAndroid []store.Count
	ByDevice  []store.Count
	TopIssues []store.Issue
	NewKey    string
}

func (s *Server) pageAppDetail(w http.ResponseWriter, r *http.Request) {
	s.renderAppDetail(w, r, "")
}

func (s *Server) renderAppDetail(w http.ResponseWriter, r *http.Request, newKey string) {
	id, ok := s.appID(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	app, err := s.Store.GetAppByID(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ctx := r.Context()
	stats, _ := s.Store.AppStats(ctx, id)
	dayCounts, _ := s.Store.ReportsPerDay(ctx, id, 30)
	byVersion, _ := s.Store.Breakdown(ctx, id, "app_version_name", 8)
	byAndroid, _ := s.Store.Breakdown(ctx, id, "android_version", 8)
	byDevice, _ := s.Store.Breakdown(ctx, id, "phone_model", 8)
	top, _ := s.Store.TopIssues(ctx, id, 10)
	s.render(w, http.StatusOK, "app_detail.html", appDetailData{
		baseData:  baseData{Title: app.Name, Authed: true, CSRF: s.csrfToken(r)},
		App:       app,
		Stats:     stats,
		Chart:     lineChart(dayCounts, 900, 240),
		ByVersion: byVersion,
		ByAndroid: byAndroid,
		ByDevice:  byDevice,
		TopIssues: top,
		NewKey:    newKey,
	})
}

func (s *Server) submitRotateKey(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	id, ok := s.appID(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	key, hash, err := keygen.NewAPIKey()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if err := s.Store.RotateAppKey(r.Context(), id, hash); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.renderAppDetail(w, r, key)
}

func (s *Server) submitDeleteApp(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	id, ok := s.appID(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := s.Store.DeleteApp(r.Context(), id); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}
