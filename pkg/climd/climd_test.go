package climd

import (
	"context"
	"os"
	"testing"
)

func TestDefaultCommand(t *testing.T) {
	old := os.Args
	os.Args = []string{"app"}
	defer func() { os.Args = old }()

	ran := ""
	cfg := AppConfig{Name: "app", Default: "serve", Commands: []Command{
		{Name: "serve", Run: func(context.Context, []string, map[string]string) error { ran = "serve"; return nil }},
		{Name: "other", Run: func(context.Context, []string, map[string]string) error { ran = "other"; return nil }},
	}}
	if err := Run(cfg, nil); err != nil || ran != "serve" {
		t.Fatalf("empty args: ran %q, err %v", ran, err)
	}
	if err := Run(cfg, []string{"other"}); err != nil || ran != "other" {
		t.Fatalf("explicit command: ran %q, err %v", ran, err)
	}
	cfg.Default = ""
	ran = ""
	if err := Run(cfg, nil); err != nil || ran != "" {
		t.Fatalf("no default: ran %q, err %v", ran, err)
	}
}
