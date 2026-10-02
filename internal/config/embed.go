package config

import _ "embed"

// defaultYAML is the shipped parameter file. It is copied to the user's
// configuration directory on first launch.
//
//go:embed defaults/clear-default.yaml
var defaultYAML []byte
