package web

import "testing"

func TestEmbeddedDashboardAssets(t *testing.T) {
	tests := []struct {
		path string
		min  int
	}{
		{path: "templates/index.html", min: 32},
		{path: "static/app.js", min: 32},
		{path: "static/styles.css", min: 32},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			data, err := Files.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) < tt.min {
				t.Fatalf("embedded %s is too small (%d bytes)", tt.path, len(data))
			}
		})
	}
}
