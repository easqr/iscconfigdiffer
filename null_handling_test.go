package differ

import (
	"testing"

	"github.com/easqr/iscconfigdiffer/v2"
)

func TestISCNullHandlingProfilesAndStrictOverride(t *testing.T) {
	left := input(t, "dev", []string{"SOURCE"}, obj("SOURCE", "same", "Invented", map[string]any{"optional": nil, "schemas": []any{map[string]any{"name": "account", "description": nil}}}))
	right := input(t, "prod", []string{"SOURCE"}, obj("SOURCE", "same", "Invented", map[string]any{"schemas": []any{map[string]any{"name": "account"}}}))
	for _, profile := range []string{"isc-identity-v2", "isc-envelope-v2", "isc-portable-v2"} {
		t.Run(profile, func(t *testing.T) {
			p := v2.Policy{Version: 2, ID: "invented", Profile: profile}
			req := Request{Mode: "drift", Left: left, Right: right}
			r := compareTest(t, req, p)
			if r.ExitCode() != 0 || r.NullHandling != "null_equals_absent" || r.Counts["equivalent"] != 2 {
				t.Fatal(r)
			}
			p.NullHandling = "strict"
			if r = compareTest(t, req, p); r.ExitCode() != 1 || r.NullHandling != "strict" {
				t.Fatal("strict override failed", r)
			}
		})
	}
}
