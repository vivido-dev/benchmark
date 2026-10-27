package main

import (
	"bytes"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestExportReplayPreservesPayload(t *testing.T) {
	oldExport, oldReplay := exportDir, replayDir
	defer func() { exportDir, replayDir = oldExport, oldReplay }()
	first, second := t.TempDir(), t.TempDir()
	exportDir, replayDir = first, ""
	payload := "text\x1b[31m界\x00\xff"
	if _, size, _, err := benchmark_data("fixture", payload, Options{}); err != nil || size != len(payload) {
		t.Fatalf("export: size=%d err=%v", size, err)
	}
	exportDir, replayDir = second, first
	if _, size, _, err := benchmark_data("fixture", "replacement must be ignored", Options{}); err != nil || size != len(payload) {
		t.Fatalf("replay: size=%d err=%v", size, err)
	}
	entries, err := os.ReadDir(first)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries: %v %v", entries, err)
	}
	replayed, err := os.ReadFile(filepath.Join(second, entries[0].Name()))
	if err != nil || !bytes.Equal(replayed, []byte(payload)) {
		t.Fatalf("replayed=%q err=%v", replayed, err)
	}
	if _, _, _, err := benchmark_data("missing fixture", "", Options{}); err == nil {
		t.Fatal("missing replay must fail before opening a terminal")
	}
}

func TestSeededPayloadIsReproducible(t *testing.T) {
	old := random
	defer func() { random = old }()
	random = rand.New(rand.NewPCG(1, 2))
	first := random_string_of_bytes(8192, ascii_printable+control_chars)
	random = rand.New(rand.NewPCG(1, 2))
	if next := random_string_of_bytes(8192, ascii_printable+control_chars); next != first {
		t.Fatal("identical seeds must generate identical payloads")
	}
}
