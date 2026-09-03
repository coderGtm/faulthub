// Command faulthub runs the FaultHub crash-report server.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"faulthub/internal/auth"
	"faulthub/internal/config"
	"faulthub/internal/ingest"
	"faulthub/internal/ratelimit"
	"faulthub/internal/store"
	"faulthub/internal/web"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hash-password":
			if err := runHashPassword(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "healthcheck":
			addr := os.Getenv("FAULTHUB_ADDR")
			if addr == "" {
				addr = ":8080"
			}
			if err := healthcheck("http://127.0.0.1" + addr + "/healthz"); err != nil {
				os.Exit(1)
			}
			return
		}
	}
	serve(os.Args[1:])
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addrFlag := fs.String("addr", "", "listen address (overrides FAULTHUB_ADDR)")
	_ = fs.Parse(args)

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if *addrFlag != "" {
		cfg.Addr = *addrFlag
	}

	adminHash := cfg.AdminPasswordHash
	if adminHash == "" {
		if adminHash, err = auth.HashPassword(cfg.AdminPassword); err != nil {
			log.Fatal(err)
		}
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "faulthub.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	ingestRate := ratelimit.New(cfg.IngestRatePerMin, cfg.IngestBurst)
	loginRate := ratelimit.New(cfg.LoginRatePerMin, 3)

	webSrv, err := web.New(st, loginRate, adminHash, cfg.CookieSecure, cfg.SessionTTL, cfg.TrustProxy)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/crash-report", &ingest.Handler{
		Store: st, Rate: ingestRate, MaxBody: cfg.MaxBodyBytes, TrustProxy: cfg.TrustProxy,
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.Handle("/", webSrv.Routes())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ingestRate.Sweep(10 * time.Minute)
				loginRate.Sweep(10 * time.Minute)
				_, _ = st.PruneSessions(context.Background(), time.Now())
			}
		}
	}()

	srv := &http.Server{Addr: cfg.Addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Fatal(err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func runHashPassword() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Password: ")
	pw, err := readLine(reader)
	if err != nil {
		return err
	}
	fmt.Print("Confirm: ")
	confirm, err := readLine(reader)
	if err != nil {
		return err
	}
	return hashPassword(pw, confirm, os.Stdout)
}

func hashPassword(pw, confirm string, out io.Writer) error {
	if pw == "" {
		return errors.New("password must not be empty")
	}
	if pw != confirm {
		return errors.New("passwords do not match")
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, h)
	return nil
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func healthcheck(url string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %d", resp.StatusCode)
	}
	return nil
}
