//go:build !xttp_pprof

package xttp

import "github.com/gofiber/fiber/v3"

const profilingBuilt = false

// mountProfiling fails loudly: without the tag pprof is not in the binary, so
// Profiling.Enabled could only be ignored.
func mountProfiling(*fiber.App, *ProfilingConfig) {
	panic("xttp: Profiling.Enabled needs a build with -tags xttp_pprof")
}
