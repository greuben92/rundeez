package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/greuben92/rundeez/view"
)

type Site struct {
	Manifest map[string]string
	mux      *http.ServeMux
}

func (site *Site) home(w http.ResponseWriter, r *http.Request) {
	data := view.HomeData{
		Manifest: site.Manifest,
		Title:    "Run Deez",
	}
	if err := view.Home(data).Render(r.Context(), w); err != nil {
		slog.Error("failed to render template", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}
}

func (site *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	site.mux.ServeHTTP(w, r)
}

func main() {
	var manifest map[string]string
	if b, err := os.ReadFile("./static/assets/manifest.json"); err == nil {
		err := json.Unmarshal(b, &manifest)
		if err != nil {
			slog.Error("failed to decode manifest.json file", "error", err)
		}
	}
	site := &Site{
		Manifest: manifest,
		mux:      http.NewServeMux(),
	}

	fs := http.FileServer(http.Dir("./static"))
	site.mux.Handle("/static/", http.StripPrefix("/static", fs))
	site.mux.HandleFunc("GET /{$}", site.home)

	server := http.Server{
		Addr:    "localhost:8080",
		Handler: site,
	}

	go func() {
		slog.Info("starting server")
		if err := server.ListenAndServe(); err != nil {
			slog.Error("listen error", "error", err)
		}
	}()

	ic := make(chan os.Signal, 1)
	signal.Notify(ic, os.Interrupt, syscall.SIGTERM)
	<-ic
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	server.Shutdown(ctx)
	slog.Info("shutting down server")
}
