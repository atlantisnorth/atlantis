package raw_test

import (
	"testing"

	"github.com/runatlantis/atlantis/server/events/yaml/raw"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
	. "github.com/runatlantis/atlantis/testing"
	yaml "gopkg.in/yaml.v2"
)

func TestAutoPlan_UnmarshalYAML(t *testing.T) {
	cases := []struct {
		description string
		input       string
		exp         raw.Autoplan
	}{
		{
			description: "omit unset fields",
			input:       "",
			exp: raw.Autoplan{
				Enabled:      nil,
				WhenModified: nil,
			},
		},
		{
			description: "all fields set",
			input: `
enabled: true
when_modified: ["something-else"]
`,
			exp: raw.Autoplan{
				Enabled:      Bool(true),
				WhenModified: []string{"something-else"},
			},
		},
		{
			description: "enabled false",
			input: `
enabled: false
when_modified: ["something-else"]
`,
			exp: raw.Autoplan{
				Enabled:      Bool(false),
				WhenModified: []string{"something-else"},
			},
		},
		{
			description: "modified elem empty",
			input: `
enabled: false
when_modified:
-
`,
			exp: raw.Autoplan{
				Enabled:      Bool(false),
				WhenModified: []string{""},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			var a raw.Autoplan
			err := yaml.UnmarshalStrict([]byte(c.input), &a)
			Ok(t, err)
			Equals(t, c.exp, a)
		})
	}
}

func TestAutoplan_Validate(t *testing.T) {
	cases := []struct {
		description string
		input       raw.Autoplan
	}{
		{
			description: "nothing set",
			input:       raw.Autoplan{},
		},
		{
			description: "when_modified empty",
			input: raw.Autoplan{
				WhenModified: []string{},
			},
		},
		{
			description: "enabled false",
			input: raw.Autoplan{
				Enabled: Bool(false),
			},
		},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			Ok(t, c.input.Validate())
		})
	}
}

func TestAutoplan_ToValid(t *testing.T) {
	cases := []struct {
		description string
		input       raw.Autoplan
		exp         valid.Autoplan
	}{
		{
			description: "nothing set",
			input:       raw.Autoplan{},
			exp: valid.Autoplan{
				Enabled:      true,
				WhenModified: []string{"**/*.tf*", "**/*.hcl"},
			},
		},
		{
			description: "when modified empty",
			input: raw.Autoplan{
				WhenModified: []string{},
			},
			exp: valid.Autoplan{
				Enabled:      true,
				WhenModified: []string{},
			},
		},
		{
			description: "enabled false",
			input: raw.Autoplan{
				Enabled: Bool(false),
			},
			exp: valid.Autoplan{
				Enabled:      false,
				WhenModified: []string{"**/*.tf*", "**/*.hcl"},
			},
		},
		{
			description: "enabled true",
			input: raw.Autoplan{
				Enabled: Bool(true),
			},
			exp: valid.Autoplan{
				Enabled:      true,
				WhenModified: []string{"**/*.tf*", "**/*.hcl"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			Equals(t, c.exp, c.input.ToValid())
		})
	}
}
