// Package web is the built SPA, embedded: pack-ui is one binary (npm run build in this directory first).
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
