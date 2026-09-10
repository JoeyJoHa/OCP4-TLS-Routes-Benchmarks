package main

import (
	"strings"
	"testing"
)

func TestRunRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		max  string
		want string
	}{
		{name: "zero max", max: "0", want: "MAX_BLOB_BYTES"},
		{name: "non integer", max: "nope", want: "MAX_BLOB_BYTES"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MAX_BLOB_BYTES", tt.max)
			err := run()
			if err == nil {
				t.Fatal("expected config error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
