package web

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"faulthub/internal/store"
)

func parseFilterDate(v string) (time.Time, bool) {
	if t, err := time.Parse("02-01-2006", v); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// pageNums returns a windowed page list with 0 marking an ellipsis gap:
// the first page, ±2 around the current page, and the last page. Small
// result sets are unabridged.
func pageNums(total, perPage, page int) []int {
	pages := (total + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	if pages <= 9 {
		out := make([]int, 0, pages)
		for i := 1; i <= pages; i++ {
			out = append(out, i)
		}
		return out
	}
	set := map[int]bool{1: true, pages: true}
	for i := page - 2; i <= page+2; i++ {
		if i >= 1 && i <= pages {
			set[i] = true
		}
	}
	var out []int
	for i := 1; i <= pages; i++ {
		if !set[i] {
			if len(out) == 0 || out[len(out)-1] != 0 {
				out = append(out, 0)
			}
			continue
		}
		out = append(out, i)
	}
	return out
}

type issuesData struct {
	baseData
	App      store.App
	Issues   []store.Issue
	Total    int
	Page     int
	PageNums []int
	BaseQ    string
	Status   string
	Q        string
	Ver      string
	OS       string
	Device   string
	From     string
	To       string
	Sort     string
	Versions []string
	Androids []string
	Devices  []string
}

func (s *Server) pageIssues(w http.ResponseWriter, r *http.Request) {
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
	q := r.URL.Query()
	f := store.IssueFilter{
		AppID:          id,
		Status:         q.Get("status"),
		Search:         q.Get("q"),
		AppVersion:     q.Get("ver"),
		AndroidVersion: q.Get("os"),
		Device:         q.Get("device"),
		Sort:           q.Get("sort"),
	}
	if f.Sort != "count" {
		f.Sort = "last"
	}
	if v := q.Get("from"); v != "" {
		if t, ok := parseFilterDate(v); ok {
			f.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, ok := parseFilterDate(v); ok {
			f.To = t.Add(24*time.Hour - time.Second)
		}
	}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	if f.Page < 1 {
		f.Page = 1
	}
	f.PerPage = 25

	issues, total, err := s.Store.ListIssues(r.Context(), f)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	versions, _ := s.Store.DistinctValues(ctx, id, "app_version_name")
	androids, _ := s.Store.DistinctValues(ctx, id, "android_version")
	devices, _ := s.Store.DistinctValues(ctx, id, "phone_model")

	baseQ := url.Values{}
	for _, k := range []string{"status", "q", "ver", "os", "device", "from", "to", "sort"} {
		if v := q.Get(k); v != "" {
			baseQ.Set(k, v)
		}
	}

	s.render(w, http.StatusOK, "issues.html", issuesData{
		baseData: baseData{Title: app.Name + " · Issues", Authed: true, CSRF: s.csrfToken(r)},
		App:      app, Issues: issues, Total: total, Page: f.Page,
		PageNums: pageNums(total, f.PerPage, f.Page), BaseQ: baseQ.Encode(),
		Status: f.Status, Q: f.Search, Ver: f.AppVersion, OS: f.AndroidVersion,
		Device: f.Device, From: q.Get("from"), To: q.Get("to"), Sort: f.Sort,
		Versions: versions, Androids: androids, Devices: devices,
	})
}

type issueDetailData struct {
	baseData
	App       store.App
	Issue     store.Issue
	Trend     []store.Count
	Latest    store.Report
	HasLatest bool
	Reports   []store.Report
	Total     int64
	Page      int
	PageNums  []int
}

func (s *Server) pageIssueDetail(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.appID(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	iid, err := strconv.ParseInt(r.PathValue("iid"), 10, 64)
	if err != nil || iid < 1 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	app, err := s.Store.GetAppByID(r.Context(), appID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	issue, err := s.Store.GetIssue(r.Context(), appID, iid)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ctx := r.Context()
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	reports, total, err := s.Store.ListIssueReports(ctx, appID, iid, page, 25)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	dayCounts, _ := s.Store.IssueReportsPerDay(ctx, iid, 30)
	var latest store.Report
	hasLatest := false
	if latestList, _, err := s.Store.ListIssueReports(ctx, appID, iid, 1, 1); err == nil && len(latestList) == 1 {
		latest, hasLatest = latestList[0], true
	}
	s.render(w, http.StatusOK, "issue_detail.html", issueDetailData{
		baseData: baseData{Title: issue.Title, Authed: true, CSRF: s.csrfToken(r)},
		App:      app, Issue: issue,
		Trend:  dayCounts,
		Latest: latest, HasLatest: hasLatest,
		Reports: reports, Total: total, Page: page,
		PageNums: pageNums(int(total), 25, page),
	})
}

func (s *Server) submitIssueStatus(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	appID, iid, ok := s.pathIDs(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	status := r.PostFormValue("status")
	if status != "open" && status != "resolved" {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := s.Store.SetIssueStatus(r.Context(), appID, iid, status); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/apps/"+strconv.FormatInt(appID, 10)+"/issues/"+strconv.FormatInt(iid, 10), http.StatusSeeOther)
}

func (s *Server) submitDeleteIssue(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	appID, iid, ok := s.pathIDs(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := s.Store.DeleteIssue(r.Context(), appID, iid); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/apps/"+strconv.FormatInt(appID, 10)+"/issues", http.StatusSeeOther)
}

type reportData struct {
	baseData
	App    store.App
	Issue  store.Issue
	Report store.Report
}

func (s *Server) pageReportDetail(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.appID(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	app, err := s.Store.GetAppByID(r.Context(), appID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rep, err := s.Store.GetReport(r.Context(), appID, r.PathValue("rid"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	issue, err := s.Store.GetIssue(r.Context(), appID, rep.IssueID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, http.StatusOK, "report_detail.html", reportData{
		baseData: baseData{Title: "Report " + rep.ID, Authed: true, CSRF: s.csrfToken(r)},
		App:      app, Issue: issue, Report: rep,
	})
}

func (s *Server) submitDeleteReport(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(w, r) {
		return
	}
	appID, ok := s.appID(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rid := r.PathValue("rid")
	rep, err := s.Store.GetReport(r.Context(), appID, rid)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := s.Store.DeleteReport(r.Context(), appID, rid); err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/apps/"+strconv.FormatInt(appID, 10)+"/issues/"+strconv.FormatInt(rep.IssueID, 10), http.StatusSeeOther)
}

func (s *Server) pathIDs(r *http.Request) (appID, iid int64, ok bool) {
	appID, ok = s.appID(r)
	if !ok {
		return 0, 0, false
	}
	iid, err := strconv.ParseInt(r.PathValue("iid"), 10, 64)
	if err != nil || iid < 1 {
		return 0, 0, false
	}
	return appID, iid, true
}
