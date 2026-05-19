package main

import "github.com/n-yokomachi/affectus/internal/viz"

func runViz(configPath, statePath string, port int) error {
	return viz.Serve(configPath, statePath, port)
}
