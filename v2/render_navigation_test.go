package v2

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestHTMLNavigationGroupsTypesAndPreservesAnchors(t *testing.T) {
	r := Report{Version: 2, Mode: "drift", Complete: true, Counts: map[string]int{"different": 2},
		Matches:   []Match{{Type: "ROLE", Key: "No displayed findings", Locations: map[string]Location{"left": {Present: true}, "right": {Present: true}}}},
		Decisions: []Decision{{Type: "SOURCE", Key: "Same name", Status: "different", Code: "value mismatch"}, {Type: "ROLE", Key: "Same name", Status: "different", Code: "value mismatch"}},
	}
	before := stable(r)
	v := r.humanView()
	if len(v.Types) != 2 || v.Types[0].Type != "ROLE" || len(v.Types[0].Groups) != 2 || v.Types[1].Type != "SOURCE" || len(v.Types[1].Groups) != 1 {
		t.Fatal("type index lost or combined objects", v.Types)
	}
	seen := map[string]bool{}
	for _, typ := range v.Types {
		for _, g := range typ.Groups {
			if seen[g.ID] {
				t.Fatal("duplicate object anchor", g.ID)
			}
			seen[g.ID] = true
		}
	}
	if len(seen) != len(v.Groups) {
		t.Fatal("type index omitted objects")
	}
	var b bytes.Buffer
	if err := r.HTML(&b); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	for _, g := range v.Groups {
		if strings.Count(s, `id="`+g.ID+`"`) != 1 || strings.Count(s, `data-group="`+g.ID+`"`) != 2 {
			t.Fatal("missing, duplicate, or inconsistent navigation target", g.ID)
		}
	}
	for _, typ := range v.Types {
		if !strings.Contains(s, `class="type-index" id="`+typ.ID+`" data-type="`+typ.ID+`"`) {
			t.Fatal("type table not collapsed by default", typ.ID)
		}
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(s, -1) {
		if ids[m[1]] {
			t.Fatal("duplicate document ID", m[1])
		}
		ids[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(s, -1) {
		if !ids[m[1]] {
			t.Fatal("broken internal link", m[1])
		}
	}
	for _, part := range []string{`aria-label="Objects by type"`, `aria-label="Object navigation"`, `id="previous"`, `id="next"`, `id="reset"`, `id="empty-index"`, `id="empty-selection"`, `<noscript>`, `No displayed findings`, `window.addEventListener('beforeprint'`, `window.addEventListener('afterprint'`} {
		if !strings.Contains(s, part) {
			t.Fatal("missing navigation or fallback", part)
		}
	}
	for _, external := range []string{`<script src=`, `<link `, `@import`, `url(http`} {
		if strings.Contains(s, external) {
			t.Fatal("report requires external assets", external)
		}
	}
	if stable(r) != before {
		t.Fatal("rendering mutated comparison data")
	}
}
