package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_ReadFirstByte_AiAgent(t *testing.T) {
	t.Run("missing_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.txt")

		_, err := ReadFirstByte(path)
		if err == nil {
			t.Fatalf("expected error for missing file")
		}
	})

	t.Run("single_byte_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sample.txt")

		if err := os.WriteFile(path, []byte("A"), 0o644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		got, err := ReadFirstByte(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "A" {
			t.Fatalf("got=%v want=%v", got, "A")
		}
	})
}

func Test_RemoveIfExists_AiAgent(t *testing.T) {
	t.Run("missing_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.txt")

		err := RemoveIfExists(path)
		if err == nil {
			t.Fatalf("expected error for missing file")
		}
	})

	t.Run("existing_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sample.txt")

		if err := os.WriteFile(path, []byte("A"), 0o644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		err := RemoveIfExists(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected file to be removed, stat err=%v", statErr)
		}
	})
}

func Test_ValidateEmail_AiAgent(t *testing.T) {
	tests := []struct {
		name string
		email string
		wantErr bool
	}{
		{name: "default_case", email: "sample@example.com", wantErr: false},
		{name: "empty_input", email: "", wantErr: true},
		{name: "invalid_input", email: "invalid-email-without-at-sign", wantErr: true},
		{name: "large_input", email: strings.Repeat("a", 11*1024) + "@example.com", wantErr: false},	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
