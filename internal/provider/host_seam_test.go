package provider_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// privateFieldAssign matches a test poking the provider's own signal or runner
// fields instead of going through the process-host seam.
var privateFieldAssign = regexp.MustCompile(`\bp\.(?:signal|runner)\s*=`)

// psNeedle is the quoted ps command string, spelled so this file does not match
// its own review grep.
var psNeedle = "\"ps\""

// TestHostSeamNoPsOutsideAdapter guards the process-host seam. Only the prochost
// adapter may name ps as a command, and no provider test may assign the private
// signal or runner fields. It walks internal and cmd like the review grep.
func TestHostSeamNoPsOutsideAdapter(t *testing.T) {
	exempt := map[string]bool{
		filepath.Join("internal", "prochost", "ps.go"): true,
	}
	var psSites, fieldSites []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join("..", "..", dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			rel, err := filepath.Rel(filepath.Join("..", ".."), path)
			if err != nil {
				return err
			}
			if exempt[rel] {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			isProviderTest := filepath.Dir(rel) == filepath.Join("internal", "provider") &&
				strings.HasSuffix(rel, "_test.go")
			for i, line := range strings.Split(string(data), "\n") {
				site := fmt.Sprintf("%s:%d", rel, i+1)
				if strings.Contains(line, psNeedle) {
					psSites = append(psSites, site)
				}
				if isProviderTest && privateFieldAssign.MatchString(line) {
					fieldSites = append(fieldSites, site)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(psSites) > 0 {
		t.Errorf("%s command outside internal/prochost/ps.go at %v", psNeedle, psSites)
	}
	if len(fieldSites) > 0 {
		t.Errorf("provider test assigns a private signal or runner field at %v", fieldSites)
	}
}
