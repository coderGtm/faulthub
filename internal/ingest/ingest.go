// Package ingest implements the ACRA crash-report HTTP contract.
package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"faulthub/internal/keygen"
	"faulthub/internal/ratelimit"
	"faulthub/internal/realip"
	"faulthub/internal/store"
)

const NoTraceHash = "no-stack-trace"

type Handler struct {
	Store      *store.Store
	Rate       *ratelimit.Limiter
	MaxBody    int64
	TrustProxy bool
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := realip.Client(r, h.TrustProxy)
	if ok, retry := h.Rate.Allow(ip); !ok {
		secs := int(retry.Seconds())
		if secs < 1 {
			secs = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limited"})
		return
	}

	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "content-type must be application/json"})
		return
	}

	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing api key"})
		return
	}
	app, err := h.Store.GetAppByKeyHash(r.Context(), keygen.HashToken(apiKey))
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid api key"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.MaxBody)
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}

	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil || body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed json"})
		return
	}

	reportID := str(body, "REPORT_ID")
	if reportID == "" || len(reportID) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "REPORT_ID required (max 128 chars)"})
		return
	}

	trace := str(body, "STACK_TRACE")
	hash := str(body, "STACK_TRACE_HASH")
	if hash == "" {
		hash = NoTraceHash
	}
	packageName := str(body, "PACKAGE_NAME")

	rep := &store.Report{
		ID:               reportID,
		AppID:            app.ID,
		InstallationID:   str(body, "INSTALLATION_ID"),
		PackageName:      packageName,
		AppVersionCode:   str(body, "APP_VERSION_CODE"),
		AppVersionName:   str(body, "APP_VERSION_NAME"),
		AndroidVersion:   str(body, "ANDROID_VERSION"),
		Brand:            str(body, "BRAND"),
		PhoneModel:       str(body, "PHONE_MODEL"),
		Product:          str(body, "PRODUCT"),
		Build:            str(body, "BUILD"),
		ThreadDetails:    str(body, "THREAD_DETAILS"),
		StackTrace:       trace,
		StackTraceHash:   hash,
		Title:            TitleFromTrace(trace, packageName),
		UserComment:      str(body, "USER_COMMENT"),
		UserEmail:        str(body, "USER_EMAIL"),
		UserAppStartDate: str(body, "USER_APP_START_DATE"),
		UserCrashDate:    str(body, "USER_CRASH_DATE"),
		ReceivedAt:       time.Now().UTC(),
		Raw:              string(bodyBytes),
	}

	inserted, err := h.Store.IngestReport(r.Context(), rep)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	code := http.StatusOK
	if inserted {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]bool{"ok": true})
}

func str(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return fmt.Sprintf("%t", t)
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	default:
		return ""
	}
}

func TitleFromTrace(trace, packageName string) string {
	if strings.TrimSpace(trace) == "" {
		return "(no stack trace)"
	}
	lines := strings.Split(trace, "\n")
	first := strings.TrimSpace(lines[0])
	frame := ""
	for _, ln := range lines[1:] {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "at ") {
			continue
		}
		if frame == "" {
			frame = t
		}
		if packageName != "" && strings.Contains(t, packageName) {
			frame = t
			break
		}
	}
	title := first
	if frame != "" {
		title = first + " at " + strings.TrimPrefix(frame, "at ")
	}
	if runes := []rune(title); len(runes) > 300 {
		title = string(runes[:300])
	}
	return title
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
