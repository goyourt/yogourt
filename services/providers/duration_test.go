package providers

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type durationHolder struct {
	MaxAge Duration `yaml:"max_age"`
}

// yaml.v3 decodes time.Duration from duration strings only. Writing the
// obvious max_age: 3600 used to stop the boot on a decoding error, so the
// field accepts both spellings now.
func TestDurationAcceptsSecondsAndDurationStrings(t *testing.T) {
	cases := []struct {
		yaml string
		want time.Duration
	}{
		{"max_age: 3600", time.Hour},
		{`max_age: "3600"`, time.Hour},
		{"max_age: 12h", 12 * time.Hour},
		{"max_age: 300ms", 300 * time.Millisecond},
		{"max_age: 0", 0},
		{"max_age: 0.5", 500 * time.Millisecond},
		{"other: 1", 0},
	}

	for _, tc := range cases {
		var holder durationHolder
		if err := yaml.Unmarshal([]byte(tc.yaml), &holder); err != nil {
			t.Errorf("%s: unexpected error %v", tc.yaml, err)
			continue
		}
		if holder.MaxAge.Duration() != tc.want {
			t.Errorf("%s: got %v, want %v", tc.yaml, holder.MaxAge.Duration(), tc.want)
		}
	}
}

func TestDurationRejectsNonsense(t *testing.T) {
	for _, in := range []string{"max_age: soon", "max_age: [1]"} {
		var holder durationHolder
		if err := yaml.Unmarshal([]byte(in), &holder); err == nil {
			t.Errorf("%s: expected a decoding error, got %v", in, holder.MaxAge.Duration())
		}
	}
}
