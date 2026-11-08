//go:build xttp_slim

package xttp

// resolveMode must not mention ModeDefault here, or the linker keeps the
// middleware. An unset Mode fails rather than silently losing the defaults.
func resolveMode(m Mode) Mode {
	if m == nil {
		panic("xttp: built with -tags xttp_slim; set Config.Mode to ModeDefault or ModeRaw")
	}
	return m
}
