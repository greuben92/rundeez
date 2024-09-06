package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

var esbuild_entry_points []string = []string{"assets/main.js", "assets/main.css"}
var esbuild_output_dir string = "./static/assets"
var esbuild_create_manifest bool = true
var esbuild_engines []esbuild.Engine = []esbuild.Engine{
	{Name: esbuild.EngineChrome, Version: "97"},
	{Name: esbuild.EngineEdge, Version: "97"},
	{Name: esbuild.EngineFirefox, Version: "96"},
	{Name: esbuild.EngineSafari, Version: "15"},
}

func create_bundler() (esbuild.BuildContext, error) {
	entry_names := "[name]"
	if esbuild_create_manifest {
		entry_names = "[name]-[hash]"
	}

	opts := esbuild.BuildOptions{
		EntryPoints:       esbuild_entry_points,
		EntryNames:        entry_names,
		Bundle:            true,
		Write:             true,
		Metafile:          true,
		Outdir:            esbuild_output_dir,
		MinifySyntax:      true,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		Sourcemap:         esbuild.SourceMapInline,
		Target:            esbuild.ES2020,
		Engines:           esbuild_engines,
		LogLimit:          6,
		LogLevel:          esbuild.LogLevelInfo,
		Plugins: []esbuild.Plugin{
			{Name: "manifest", Setup: esbuild_manifest_plugin},
		},
	}

	ctx, err := esbuild.Context(opts)
	if err != nil {
		slog.Error("esbuild context error", "error", err.Error())
		return nil, errors.New("esbuild error")
	}

	return ctx, nil
}

type metafile struct {
	Outputs map[string]metafile_output `json:"outputs"`
}

type metafile_output struct {
	EntryPoint string `json:"entryPoint"`
}

func esbuild_manifest_plugin(pb esbuild.PluginBuild) {
	if !esbuild_create_manifest {
		return
	}

	pb.OnEnd(func(result *esbuild.BuildResult) (esbuild.OnEndResult, error) {
		if result.Errors != nil {
			return esbuild.OnEndResult{}, nil
		}

		mfile := new(metafile)
		if err := json.Unmarshal([]byte(result.Metafile), mfile); err != nil {
			slog.Error("esbuild metafile", "error", err)
		}

		if mfile.Outputs != nil {
			manifest := map[string]string{}
			for filename, output := range mfile.Outputs {
				if output.EntryPoint != "" {
					manifest[output.EntryPoint] = filename
				}
			}

			m, _ := json.Marshal(manifest)
			err := os.WriteFile(esbuild_output_dir+"/manifest.json", m, 0644)
			if err != nil {
				slog.Error("failed to write manifest file", "error", err)
			}
		}

		return esbuild.OnEndResult{}, nil
	})
}
