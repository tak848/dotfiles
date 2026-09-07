// cc-model-router: Claude Code の ANTHROPIC_BASE_URL に立てる、モデル名で振り分けるリバースプロキシ。
//
// claudep（dot_zsh/functions/claudep.zsh）が使う。1 つの Claude Code セッションの中で Claude と Codex（gpt-*）を
// 混ぜるための最小のルーターで、リクエスト本文の "model" だけを見て転送先を決める。
//
//   - gpt-* など Codex のモデル名 → CLIProxyAPI（127.0.0.1:8317）。Claude サブスクの OAuth トークンが
//     CLIProxyAPI（とそのログ）に渡らないよう Authorization / x-api-key を落とし、ダミーの Bearer に差し替える
//   - それ以外（claude-*、model が無い GET /v1/models 等）→ api.anthropic.com。ヘッダも本文も一切触らない。
//     Claude Code は ANTHROPIC_BASE_URL だけ設定して認証変数を設定しなければサブスクの OAuth で認証し続ける仕様で、
//     その条件は gateway が Authorization と anthropic-beta（OAuth の capability）をそのまま転送することなので、
//     ここでは Host 以外の書き換えをしない
//
// ログには method / path / model / 転送先 / status / 所要時間しか出さない。ヘッダや本文は決して出さない。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type config struct {
	anthropic     *url.URL
	codex         *url.URL
	codexPrefixes []string
	codexToken    string
	logger        *log.Logger
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8318", "listen address")
	anthropic := flag.String("anthropic", "https://api.anthropic.com", "upstream for Claude models (verbatim pass-through)")
	codex := flag.String("codex", "http://127.0.0.1:8317", "upstream for Codex models (CLIProxyAPI)")
	prefixes := flag.String("codex-prefixes", "gpt-", "comma-separated, case-insensitive model name prefixes routed to -codex")
	codexToken := flag.String("codex-token", "unused", "bearer token sent to -codex instead of the client's credential")
	flag.Parse()

	logger := log.New(os.Stderr, "", log.LstdFlags)

	anthropicURL, err := url.Parse(*anthropic)
	if err != nil {
		logger.Fatalf("invalid -anthropic: %v", err)
	}
	codexURL, err := url.Parse(*codex)
	if err != nil {
		logger.Fatalf("invalid -codex: %v", err)
	}

	cfg := config{
		anthropic:     anthropicURL,
		codex:         codexURL,
		codexPrefixes: splitPrefixes(*prefixes),
		codexToken:    *codexToken,
		logger:        logger,
	}

	srv := &http.Server{
		Addr:    *listen,
		Handler: newHandler(cfg),
		// SSE のレスポンスは 1 ターンで数分続くので WriteTimeout は付けない。
		// ReadHeaderTimeout だけ、接続を開いて何も送らないクライアント対策に置く
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Printf("listening on %s (claude -> %s, %s -> %s)", *listen, anthropicURL, strings.Join(cfg.codexPrefixes, ","), codexURL)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("serve: %v", err)
	}
}

func splitPrefixes(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

type router struct {
	cfg       config
	anthropic *httputil.ReverseProxy
	codex     *httputil.ReverseProxy
}

func newHandler(cfg config) http.Handler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// 上流がヘッダを返すまでの時間に上限を付けない。reasoning が長いと最初の byte まで数分かかる
	transport.ResponseHeaderTimeout = 0

	r := &router{cfg: cfg}
	r.anthropic = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Director と違い Rewrite は X-Forwarded-For を足さない。SetURL は Host も書き換える。
			// それ以外のヘッダ（Authorization / x-api-key / anthropic-beta / anthropic-version）は素通し
			pr.SetURL(cfg.anthropic)
		},
		Transport:     transport,
		FlushInterval: -1, // SSE をバッファせず即時 flush
		ErrorHandler:  r.errorHandler("anthropic"),
	}
	r.codex = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(cfg.codex)
			// Claude サブスクの OAuth トークンを CLIProxyAPI に渡さない。
			// CLIProxyAPI は api-keys が空だと Bearer を無視するが、ログに残る可能性を潰すため必ず差し替える
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("X-Api-Key")
			pr.Out.Header.Set("Authorization", "Bearer "+cfg.codexToken)
		},
		Transport:     transport,
		FlushInterval: -1,
		ErrorHandler:  r.errorHandler("codex"),
	}

	mux := http.NewServeMux()
	// 1 行目は生存確認、2 行目以降は起動時に固定された転送先。claudep はこれを期待値と照合し、
	// 設定変更後に古いプロセスが残っている（別の CLIProxyAPI ポートに転送している）ことを検出する
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "ok\ncodex=%s\nanthropic=%s\n", cfg.codex, cfg.anthropic)
	})
	mux.Handle("/", r)
	return mux
}

func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	model, err := peekModel(req)
	if err != nil {
		r.cfg.logger.Printf("%s %s read body: %v", req.Method, req.URL.Path, err)
		http.Error(w, "cc-model-router: failed to read request body", http.StatusBadRequest)
		return
	}

	target := "anthropic"
	proxy := r.anthropic
	if r.isCodexModel(model) {
		target = "codex"
		proxy = r.codex
	}

	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	proxy.ServeHTTP(sw, req)

	r.cfg.logger.Printf("%s %s model=%q -> %s %d %s", req.Method, req.URL.Path, model, target, sw.status, time.Since(start).Round(time.Millisecond))
}

func (r *router) isCodexModel(model string) bool {
	m := strings.ToLower(model)
	for _, p := range r.cfg.codexPrefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

func (r *router) errorHandler(target string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, req *http.Request, err error) {
		r.cfg.logger.Printf("%s %s -> %s upstream error: %v", req.Method, req.URL.Path, target, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "api_error",
				"message": fmt.Sprintf("cc-model-router: %s upstream unreachable: %v", target, err),
			},
		})
	}
}

// peekModel は本文を全部読んで "model" だけ取り出し、本文を元に戻す。本文が無い・JSON でない・model が無い場合は
// 空文字を返す（呼び出し側は anthropic に流す）。本文自体はバイト列としてそのまま上流へ渡す。
func peekModel(req *http.Request) (string, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return "", nil
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return "", err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return "", nil
	}
	return probe.Model, nil
}

// statusWriter はログ用に status code を控える。ReverseProxy は http.NewResponseController 経由で Flush するので
// Unwrap を実装して元の ResponseWriter に辿れるようにしておく（これが無いと SSE が flush されない）。
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
