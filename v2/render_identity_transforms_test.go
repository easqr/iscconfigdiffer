package v2

import (
	"reflect"
	"testing"
)

func TestIdentityTransformSectionsUseCapturedNames(t *testing.T) {
	decision := func(names ...string) humanDecision {
		d := humanDecision{Category: "identity-transforms", Status: "different", DecisionCount: 1}
		for _, name := range names {
			d.Locations = append(d.Locations, humanLocation{Context: []ItemContext{{
				Pointer: "/identityAttributeConfig/attributeTransforms/0",
				Fields:  []ContextField{{Label: "Identity attribute", Value: name}},
			}}})
		}
		return d
	}
	g := humanGroup{ID: "object-1", Decisions: []humanDecision{
		decision("email", "email"), decision("displayName", "displayName"),
		decision("email", "email"), decision(), decision("left-name", "right-name"),
		{Category: "other", Status: "excluded", DecisionCount: 1},
	}}
	sections := identityTransformSections(g)
	titles := []string{}
	counts := []int{}
	for _, s := range sections {
		titles = append(titles, s.Title)
		counts = append(counts, s.DecisionCount)
	}
	if !reflect.DeepEqual(titles, []string{"Identity attribute transforms", "Identity attribute transform: displayName", "Identity attribute transform: email"}) || !reflect.DeepEqual(counts, []int{2, 1, 2}) {
		t.Fatal("decisions missing, combined incorrectly, or names inferred", sections)
	}
	for i, j := 0, len(g.Decisions)-1; i < j; i, j = i+1, j-1 {
		g.Decisions[i], g.Decisions[j] = g.Decisions[j], g.Decisions[i]
	}
	for i, s := range identityTransformSections(g) {
		if s.Title != titles[i] || s.DecisionCount != counts[i] || s.ID != sections[i].ID {
			t.Fatal("section grouping depends on decision order")
		}
	}
}
