//go:build xttp_slim

package xttp

import "testing"

func TestSlimRefusesUnsetMode(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New with nil Mode did not panic in an xttp_slim build")
		}
	}()
	New(Config{})
}

func TestSlimKeepsExplicitDefault(t *testing.T) {
	if New(Config{Mode: ModeDefault}).headers == nil {
		t.Fatal("no headers")
	}
}
