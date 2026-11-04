// wiki serves the interview-prep markdown wiki as HTML and exposes Prometheus metrics.
// Fault injection (error rate, latency, memory leak) is built in, so SLO alerts and
// Kubernetes failure modes can be reproduced on purpose.
//
//	wiki            — HTTP server
//	wiki loadgen    — steady traffic against TARGET, so the SLI graphs have data
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

var (
	requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests served, by route and status code.",
	}, []string{"route", "code"})
	duration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request latency, by route.",
		// 0.3 is the latency SLO threshold in the Helm chart: a bucket must exist exactly there.
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.3, 0.5, 1, 2.5, 5},
	}, []string{"route"})
	leakedBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "wiki_leaked_bytes",
		Help: "Memory held on purpose by the LEAK_MB_PER_SEC fault.",
	})
)

// Topic is one markdown file rendered to HTML.
type Topic struct {
	File, Title string
	HTML        template.HTML
}

// Site holds both languages; it is read-only after load.
type Site struct {
	topics map[string][]Topic // lang ("ru", "en") -> topics sorted by file name
	byFile map[string]Topic   // "ru/01-kubernetes.md" -> topic
}

// Faults are changed at runtime through /admin/fault, so they are guarded by a mutex.
type Faults struct {
	mu        sync.RWMutex
	ErrorRate float64       // share of requests answered with 500
	SlowRate  float64       // share of requests delayed by Latency
	Latency   time.Duration // extra delay for slow requests
}

func (f *Faults) get() (float64, float64, time.Duration) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ErrorRate, f.SlowRate, f.Latency
}

func (f *Faults) String() string {
	e, s, l := f.get()
	return fmt.Sprintf("error_rate=%g slow_rate=%g latency=%s\n", e, s, l)
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) > 1 && os.Args[1] == "loadgen" {
		loadgen(env("TARGET", "http://localhost:8080"), envFloat("RPS", 5))
		return
	}
	dir := env("CONTENT_DIR", "/content")
	site, err := loadSite(dir)
	if err != nil {
		slog.Error("cannot load content", "dir", dir, "err", err)
		os.Exit(1)
	}
	faults := &Faults{
		ErrorRate: envFloat("FAULT_ERROR_RATE", 0),
		SlowRate:  envFloat("FAULT_SLOW_RATE", 0),
		Latency:   envDuration("FAULT_LATENCY", 0),
	}
	if mb := envFloat("LEAK_MB_PER_SEC", 0); mb > 0 {
		go leak(mb)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", instrument("index", faults, site.index("ru")))
	mux.Handle("GET /en/{$}", instrument("index", faults, site.index("en")))
	mux.Handle("GET /t/{file}", instrument("topic", faults, site.topic("ru")))
	mux.Handle("GET /en/t/{file}", instrument("topic", faults, site.topic("en")))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ready") })
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("/admin/fault", faults.handle)

	srv := &http.Server{Addr: env("ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("listening", "addr", srv.Addr, "topics_ru", len(site.topics["ru"]), "topics_en", len(site.topics["en"]),
			"faults", strings.TrimSpace(faults.String()))
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// SIGTERM from the kubelet: finish in-flight requests instead of dropping them.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// instrument counts requests and latency per route and applies the configured faults.
func instrument(route string, f *Faults, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		errorRate, slowRate, latency := f.get()
		if rand.Float64() < slowRate {
			time.Sleep(latency)
		}
		if rand.Float64() < errorRate {
			http.Error(rec, "injected fault", http.StatusInternalServerError)
		} else {
			next.ServeHTTP(rec, r)
		}
		requests.WithLabelValues(route, strconv.Itoa(rec.code)).Inc()
		duration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// handle: GET shows the faults, POST /admin/fault?error_rate=0.2&slow_rate=0.5&latency=400ms changes them.
// Only this pod changes — with several replicas, use the Helm values instead.
func (f *Faults) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		q := r.URL.Query()
		f.mu.Lock()
		if v, err := strconv.ParseFloat(q.Get("error_rate"), 64); err == nil {
			f.ErrorRate = v
		}
		if v, err := strconv.ParseFloat(q.Get("slow_rate"), 64); err == nil {
			f.SlowRate = v
		}
		if v, err := time.ParseDuration(q.Get("latency")); err == nil {
			f.Latency = v
		}
		f.mu.Unlock()
		slog.Info("faults changed", "faults", strings.TrimSpace(f.String()))
	}
	fmt.Fprint(w, f.String())
}

// leak holds on to mb megabytes more every second, until the container hits its memory limit.
func leak(mb float64) {
	var held [][]byte
	for range time.Tick(time.Second) {
		chunk := bytes.Repeat([]byte{1}, int(mb*1024*1024)) // filled, so the pages are really resident
		held = append(held, chunk)
		leakedBytes.Add(float64(len(chunk)))
	}
}

var h1 = regexp.MustCompile(`(?m)^#\s+(.+)$`)

func loadSite(dir string) (*Site, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	site := &Site{topics: map[string][]Topic{}, byFile: map[string]Topic{}}
	for lang, sub := range map[string]string{"ru": "", "en": "en"} {
		files, err := filepath.Glob(filepath.Join(dir, sub, "*.md"))
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			src, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var buf bytes.Buffer
			if err := md.Convert(src, &buf); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			t := Topic{File: filepath.Base(path), Title: filepath.Base(path), HTML: template.HTML(buf.String())}
			if m := h1.FindSubmatch(src); m != nil {
				t.Title = string(m[1])
			}
			site.topics[lang] = append(site.topics[lang], t)
			site.byFile[lang+"/"+t.File] = t
		}
		sort.Slice(site.topics[lang], func(i, j int) bool { return site.topics[lang][i].File < site.topics[lang][j].File })
	}
	if len(site.topics["ru"]) == 0 {
		return nil, fmt.Errorf("no *.md files in %s", dir)
	}
	return site, nil
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
body{font:16px/1.6 system-ui,sans-serif;max-width:900px;margin:0 auto;padding:0 16px 48px;color:#1d1d1f;background:#fff}
a{color:#0b63c5}pre{background:#f4f4f6;padding:12px;overflow-x:auto}code{font-size:.9em}
table{border-collapse:collapse;display:block;overflow-x:auto}td,th{border:1px solid #ddd;padding:4px 8px}
nav{display:flex;gap:16px;padding:12px 0;border-bottom:1px solid #eee;margin-bottom:16px}
@media (prefers-color-scheme:dark){body{background:#151517;color:#e6e6e6}pre{background:#222}a{color:#6cb2ff}td,th{border-color:#333}}
</style></head><body>
<nav><a href="{{.Home}}">{{if eq .Lang "en"}}All topics{{else}}Все темы{{end}}</a>
<a href="{{.Other}}">{{if eq .Lang "en"}}RU{{else}}EN{{end}}</a></nav>
{{if .Topics}}<h1>{{.Title}}</h1><ul>{{range .Topics}}<li><a href="{{$.Home}}t/{{.File}}">{{.Title}}</a></li>{{end}}</ul>
{{else}}{{.Body}}{{end}}
</body></html>`))

type pageData struct {
	Lang, Title, Home, Other string
	Topics                   []Topic
	Body                     template.HTML
}

func prefix(lang string) string {
	if lang == "en" {
		return "/en/"
	}
	return "/"
}

func otherLang(lang string) string {
	if lang == "en" {
		return "ru"
	}
	return "en"
}

func (s *Site) index(lang string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		title := "Подготовка к собеседованиям"
		if lang == "en" {
			title = "Interview prep"
		}
		render(w, pageData{Lang: lang, Title: title, Home: prefix(lang), Other: prefix(otherLang(lang)),
			Topics: s.topics[lang]})
	})
}

func (s *Site) topic(lang string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, ok := s.byFile[lang+"/"+r.PathValue("file")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		render(w, pageData{Lang: lang, Title: t.Title, Home: prefix(lang),
			Other: prefix(otherLang(lang)) + "t/" + t.File, Body: t.HTML})
	})
}

func render(w http.ResponseWriter, d pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, d); err != nil {
		slog.Error("render", "err", err)
	}
}

// loadgen requests the index and random topics at a steady rate and logs the share of failures.
func loadgen(target string, rps float64) {
	client := &http.Client{Timeout: 5 * time.Second}
	paths := []string{"/", "/en/", "/t/01-kubernetes.md", "/t/10-sre-observability.md", "/en/t/20-english.md"}
	slog.Info("loadgen started", "target", target, "rps", rps)
	var ok, failed int
	for range time.Tick(time.Duration(float64(time.Second) / rps)) {
		resp, err := client.Get(target + paths[rand.IntN(len(paths))])
		if err == nil {
			resp.Body.Close()
		}
		if err != nil || resp.StatusCode >= 500 {
			failed++
		} else {
			ok++
		}
		if ok+failed == int(rps*60) {
			slog.Info("loadgen last minute", "ok", ok, "failed", failed)
			ok, failed = 0, 0
		}
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(key))
	if err != nil {
		return def
	}
	return v
}
