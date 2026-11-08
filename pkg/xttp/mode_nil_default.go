//go:build !xttp_slim

package xttp

func resolveMode(m Mode) Mode {
	if m == nil {
		return ModeDefault
	}
	return m
}
