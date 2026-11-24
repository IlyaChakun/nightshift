// Package schemas embeds the JSON Schemas for configs and role results so the
// binary validates them without reading files at runtime.
package schemas

import "embed"

// FS holds every *.json schema in this directory.
//
//go:embed *.json
var FS embed.FS
