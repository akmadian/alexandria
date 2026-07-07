package domain

import "testing"

func TestSlugify(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"Nature", "nature"},
		{"  leading trailing  ", "leading-trailing"},
		{"multi   space\tcollapse", "multi-space-collapse"},
		{"café", "café"},
		{"赤", "赤"},
		{"Animals | Cats", "animals-|-cats"},
		{"", ""},
	} {
		if got := Slugify(tc.input); got != tc.want {
			t.Errorf("Slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
