// Package portal contains deployable, trusted portal resources.
package portal

import "embed"

// Resources contains the initial template scaffold, not a completed business website.
//
//go:embed themes admin-dist
var Resources embed.FS
