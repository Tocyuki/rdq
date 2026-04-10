package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
)

func Run(port int) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	fsys, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(fsys)))

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("rdq gui: listening on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}
