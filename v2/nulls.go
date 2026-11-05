package v2

// These semantics apply to content properties only. Metadata, identity components
// and array membership retain their strict representations.
func cNullHandling(c *Compiled) string {
	if c == nil {
		return "strict"
	}
	return c.policy.NullHandling
}
func (x *execution) nullProperties(f frame) bool {
	return x.c.policy.NullHandling == "null_equals_absent" && f.space == "content" && f.property
}
func (x *execution) comparisonValue(space string, value any) any {
	if x.c.policy.NullHandling != "null_equals_absent" || space != "content" {
		return value
	}
	return withoutNullProperties(value)
}
func (x *execution) statesEqual(f frame, a, b slot) bool {
	if a.present == b.present && (!a.present || equal(a.value, b.value)) {
		return true
	}
	if a.unavailable || b.unavailable {
		return false
	}
	if a.present != b.present {
		return x.nullProperties(f) && (!a.present || a.value == nil) && (!b.present || b.value == nil)
	}
	if !a.present {
		return true
	}
	return equal(x.comparisonValue(f.space, a.value), x.comparisonValue(f.space, b.value))
}

// Build comparison-only copies; never alter input values or report locations.
func withoutNullProperties(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			if item != nil {
				out[k] = withoutNullProperties(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = withoutNullProperties(item)
		}
		return out
	default:
		return value
	}
}
