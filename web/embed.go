package web

import "embed"

// Files holds the dashboard HTML, CSS, and JS.
//
//go:embed templates/index.html static/*
var Files embed.FS
