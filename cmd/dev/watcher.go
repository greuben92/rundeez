package main

import (
	"io/fs"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (app *app_t) watch_dir(path string) error {
	return filepath.Walk(path, func(path string, finfo fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if finfo.IsDir() {
			// Skip hidden directories i.e .git
			if finfo.Name()[0] == '.' {
				return filepath.SkipDir
			}

			for _, epath := range app.exclude_dirs {
				if path == epath {
					slog.Warn("skipping directory", "path", path)
					return filepath.SkipDir
				}
			}

			slog.Info("watching directory", "path", path)
			app.watcher.Add(path)
			app.watched_dirs_mutex.Lock()
			app.watched_dirs = append(app.watched_dirs, path)
			app.watched_dirs_mutex.Unlock()
		}

		return nil
	})
}

func (app *app_t) process_fsevent() {
	var (
		wait_for = 100 * time.Millisecond
		mu       sync.Mutex
		timers   = make(map[string]*time.Timer)
		runEvent = func(event fsnotify.Event, cb func(event fsnotify.Event)) {
			mu.Lock()
			cb(event)
			delete(timers, event.Name)
			mu.Unlock()
		}
	)

	for {
		select {
		case err, ok := <-app.watcher.Errors:
			if !ok { // Watcher is closed
				return
			}
			slog.Error(err.Error())

		case event, ok := <-app.watcher.Events:
			if !ok { // Watcher is closed
				return
			}

			var cb func(fsnotify.Event)

			if event.Has(fsnotify.Create) {
				cb = app.fsevent_create_handler
			} else if event.Has(fsnotify.Remove) {
				cb = app.fsevent_remove_handler
			} else if event.Has(fsnotify.Write) {
				cb = app.fsevent_write_handler
			}

			if cb == nil {
				break
			}

			mu.Lock()
			t, ok := timers[event.Name]
			mu.Unlock()

			if !ok {
				t = time.AfterFunc(math.MaxInt64, func() { runEvent(event, cb) })
				t.Stop()

				mu.Lock()
				timers[event.Name] = t
				mu.Unlock()
			}

			t.Reset(wait_for)
		}
	}
}

func (app *app_t) fsevent_create_handler(event fsnotify.Event) {
	info, err := os.Lstat(event.Name)
	if err != nil {
		return
	}

	if info.IsDir() {
		err = app.watch_dir(event.Name)
		if err != nil {
			slog.Error("failed to watch new directory", "error", err)
		}
	}
}

func (app *app_t) fsevent_remove_handler(event fsnotify.Event) {
	index, ok := slices.BinarySearch(app.watched_dirs, event.Name)
	if !ok {
		return
	}
	app.watcher.Remove(event.Name)
	app.watched_dirs_mutex.Lock()
	app.watched_dirs = slices.Delete(app.watched_dirs, index, index+1)
	app.watched_dirs_mutex.Unlock()
}

func (app *app_t) fsevent_write_handler(event fsnotify.Event) {
	ext := filepath.Ext(event.Name)
	switch ext {
	case ".js", ".css":
		app.run_bundler()
		// Assuming manifest.json file is read on startup, restart the server to serve newly built assets
		if esbuild_create_manifest {
			restart_server(app, event.Name)
		}
	case ".go":
		restart_server(app, event.Name)
	}
}

func restart_server(app *app_t, file string) {
	slog.Info("restarting server...", "file", file)
	err := app.server_task.restart()
	if err != nil {
		slog.Info("failed to restart server", "error", err)
	}

	exec.Command("templ", "generate", "--notify-proxy").Output()
}
