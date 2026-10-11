package v1alpha1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Kubebuilder markers become CRD validation rules, and CEL rejects typographic quotes. A
// curly quote once replaced two single quotes in a rule and broke every CRD install, so markers must be ASCII.
func TestMarkersAreASCII(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if !strings.Contains(line, "+kubebuilder:") {
				continue
			}
			if strings.Contains(line, "''") {
				t.Errorf("%s:%d: '' in a kubebuilder marker; gofmt rewrites it, use size() instead", f, i+1)
			}
			for _, r := range line {
				if r > 127 {
					t.Errorf("%s:%d: non-ASCII %q in a kubebuilder marker", f, i+1, r)
					break
				}
			}
		}
	}
}
