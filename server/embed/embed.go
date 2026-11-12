// Package embed holds the files the plugin ships: the skill and its
// templates, embedded into the binary so byline-mcp carries them without
// reading the repo. server/scripts/embed.sh keeps the committed copies under
// this package in sync with their sources.
package embed

import "embed"

//go:embed skills templates rules
var Files embed.FS
