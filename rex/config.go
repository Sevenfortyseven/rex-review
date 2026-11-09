// Package rexconfig contains the Lua action installed alongside Rex Review.
package rexconfig

import _ "embed"

//go:embed config.lua
var Lua []byte
