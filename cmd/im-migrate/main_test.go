package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgumentsRejectUnsafeOrIncompleteCommands(t *testing.T) {
	for _, args := range [][]string{
		{}, {"down"}, {"drop"}, {"baseline"}, {"repair", "-version", "6"},
		{"baseline", "-version", "-1"}, {"up", "-version", "2"},
		{"status", "extra"}, {"status", "-config"}, {"status", "-unknown"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if err := run(context.Background(), args, &bytes.Buffer{}); err == nil {
				t.Fatal("invalid arguments accepted")
			}
		})
	}
}

func TestHelpNeedsNoConfigurationOrDatabase(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "baseline") {
		t.Fatal(out.String())
	}
}

func TestOnlyMySQLConfigurationIsRequired(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "mysql-only.yaml")
	if err := os.WriteFile(filename, []byte("mysql:\n  host: localhost\n  port: 0\n  user: tester\n  database: isolated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"status", "-config", filename}, {"-config", filename, "status"}, {"status", "-config=" + filename},
	} {
		err := run(context.Background(), args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "mysql host, port") {
			t.Fatalf("error = %v", err)
		}
	}
}
