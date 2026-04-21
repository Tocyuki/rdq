package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
)

// Options configures a `rdq gui` run. Values are propagated from command-line
// flags + the per-profile state.json into the session so the SPA can reload
// without losing its context.
type Options struct {
	// Port is the TCP port to bind on 127.0.0.1. 0 means "pick a free
	// port" which is primarily useful from tests.
	Port int

	// NoOpen disables the automatic browser launch on startup.
	NoOpen bool

	// Dev adds http://localhost:5173 / http://127.0.0.1:5173 (the Vite
	// dev server) to the allowed-origin list so frontend hot-reload works
	// without having to run the Go binary through Vite's proxy in reverse.
	Dev bool

	// Initial* seed the session store before the first GET /api/session.
	InitialProfile         string
	InitialCluster         string
	InitialSecret          string
	InitialDatabase        string
	InitialBedrockModel    string
	InitialBedrockLanguage string
}

// Run starts the HTTP server synchronously and returns when it shuts down.
// Cancelling ctx triggers a graceful shutdown; when ctx is nil the server
// runs until the process is killed.
func Run(ctx context.Context, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}

	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return fmt.Errorf("mount embedded dist: %w", err)
	}
	if err := verifyFrontendEmbed(distFS); err != nil {
		return err
	}

	seed := SessionDTO{
		Profile:         opts.InitialProfile,
		Cluster:         opts.InitialCluster,
		Secret:          opts.InitialSecret,
		Database:        opts.InitialDatabase,
		BedrockModel:    opts.InitialBedrockModel,
		BedrockLanguage: opts.InitialBedrockLanguage,
	}
	seed = LoadFromState(seed)

	// History is best-effort — a broken file should not stop the server
	// from booting. Failure is logged and the SPA simply gets an empty
	// history list.
	hist, err := history.New()
	if err != nil {
		log.Printf("rdq gui: history disabled: %v", err)
		hist = nil
	}

	deps := handlerDeps{
		session:        newSessionStore(seed),
		awsCache:       newAWSCache(),
		history:        hist,
		distFS:         distFS,
		allowedOrigins: allowedOrigins(opts.Port, opts.Dev),
	}

	return serve(ctx, opts, buildRouter(deps))
}

// serve binds a 127.0.0.1 listener, launches a browser if requested, and
// blocks on the http.Server until ctx is done.
func serve(ctx context.Context, opts Options, handler http.Handler) error {
	addr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	// Resolve the actual listener address so Port:0 printouts and browser
	// launches use the real port.
	actual := ln.Addr().(*net.TCPAddr)
	url := fmt.Sprintf("http://127.0.0.1:%d", actual.Port)
	log.Printf("rdq gui: listening on %s", url)

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	if !opts.NoOpen {
		go openBrowser(url)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// allowedOrigins builds the origin allow-list for the check-origin
// middleware. The production allow-list is the localhost hostname + IP on the
// same port; development adds the Vite dev server.
func allowedOrigins(port int, dev bool) []string {
	out := []string{
		fmt.Sprintf("http://127.0.0.1:%d", port),
		fmt.Sprintf("http://localhost:%d", port),
	}
	if dev {
		out = append(out,
			"http://127.0.0.1:5173",
			"http://localhost:5173",
		)
	}
	return out
}

// verifyFrontendEmbed reports a helpful error when the embedded SPA is
// missing. Tagged releases carry the compiled frontend bundle in their
// tag commit (see .github/workflows/tagpr.yml), so `go install @latest`
// ships a working `rdq gui`. Non-release builds — `go install @main`,
// `go install ...@<sha>`, or a local `go build` without
// `make frontend-build` — only have the tracked
// internal/server/dist/.gitkeep placeholder, and without this check the
// server would start happily and serve a directory listing showing just
// ".gitkeep", which looks broken rather than self-explanatory.
func verifyFrontendEmbed(distFS fs.FS) error {
	if _, err := fs.Stat(distFS, "index.html"); err == nil {
		return nil
	}
	return errors.New(
		"rdq gui: the embedded frontend is missing.\n" +
			"This binary was built from a non-release commit that does not carry the frontend bundle.\n" +
			"Fix it by installing rdq in one of these ways:\n" +
			"  - go install github.com/Tocyuki/rdq/cmd/rdq@latest\n" +
			"  - download a release tarball from https://github.com/Tocyuki/rdq/releases\n" +
			"  - clone the repo and run `make build` (requires Node.js and npm)\n" +
			"The `rdq` (TUI) and `rdq exec` / `rdq tui` subcommands work without the frontend; only `rdq gui` needs it.")
}

// openBrowser tries to launch the user's default browser pointed at url. It
// silently falls through on unsupported platforms — the server still works,
// the user just has to click the printed URL.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Run()
	}
}
