package main

import (
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/fsnotify/fsnotify"
)

var exclude_dirs_from_watch []string = []string{"./static/assets"}

type task_t struct {
	mutex sync.Mutex
	cmd   *exec.Cmd
	prog  string
	args  []string
}

func create_cmd(prog string, args ...string) *exec.Cmd {
	cmd := exec.Command(prog, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func start_task(prog string, args ...string) (*task_t, error) {
	cmd := create_cmd(prog, args...)
	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	return &task_t{
		cmd:  cmd,
		prog: prog,
		args: args,
	}, nil
}

func (task *task_t) restart() error {
	err := task.stop()
	if err != nil {
		return err
	}
	task.cmd = create_cmd(task.prog, task.args...)
	err = task.cmd.Start()
	return err
}

func (task *task_t) stop() error {
	task.mutex.Lock()
	defer task.mutex.Unlock()

	pgid, err := syscall.Getpgid(task.cmd.Process.Pid)
	if err != nil {
		return err
	}

	return syscall.Kill(-pgid, syscall.SIGTERM)
}

type app_t struct {
	root_path          string
	watcher            *fsnotify.Watcher
	watched_dirs       []string
	exclude_dirs       []string
	bundler            esbuild.BuildContext
	watched_dirs_mutex sync.Mutex
	templ_task         *task_t
	server_task        *task_t
}

func setup() (*app_t, error) {
	path, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	bundler, err := create_bundler()
	if err != nil {
		return nil, err
	}

	return &app_t{
		root_path:    path,
		watcher:      watcher,
		exclude_dirs: exclude_dirs_from_watch,
		bundler:      bundler,
	}, nil
}

func (app *app_t) run_bundler() {
	slog.Info("building assets")
	result := app.bundler.Rebuild()
	if result.Errors != nil {
		slog.Error("esbuild error", "errors", result.Errors)
		return
	}
	if result.Warnings != nil {
		slog.Warn("esbuild warnings", "warnings", result.Warnings)
	}
}

func main() {
	var err error

	if err := os.MkdirAll(esbuild_output_dir, 0750); err != nil {
		slog.Error("failed to create esbuild_output_dir", "error", err)
	}

	app, err := setup()
	if err != nil {
		slog.Error("failed to setup app", "error", err)
		os.Exit(1)
	}

	app.run_bundler()
	app.server_task, err = start_task("go", "run", "./cmd/server")
	if err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	app.templ_task, err = start_task(
		"templ", "generate", "--watch",
		"--proxy", "http://localhost:8080",
	)
	if err != nil {
		slog.Error("failed to start templ watcher", "error", err)
		os.Exit(1)
	}

	go app.process_fsevent()
	app.watch_dir(app.root_path)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	slog.Info("stopping templ generator")
	app.templ_task.stop()
	slog.Info("stopping server")
	app.server_task.stop()
}
