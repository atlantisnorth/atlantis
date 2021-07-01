package raw

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
)

// WorkflowHook represents a single action/command to perform. In YAML,
// it can be set as
// A map for a custom run commands:
//    - run: my custom command
type WorkflowHook struct {
	StringVal map[string]string
}

func (s *WorkflowHook) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return s.unmarshalGeneric(unmarshal)
}

func (s WorkflowHook) MarshalYAML() (interface{}, error) {
	return s.marshalGeneric()
}

func (s *WorkflowHook) UnmarshalJSON(data []byte) error {
	return s.unmarshalGeneric(func(i interface{}) error {
		return json.Unmarshal(data, i)
	})
}

func (s *WorkflowHook) MarshalJSON() ([]byte, error) {
	out, err := s.marshalGeneric()
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (s WorkflowHook) Validate() error {
	runStep := func(value interface{}) error {
		elem := value.(map[string]string)
		var keys []string
		for k := range elem {
			keys = append(keys, k)
		}
		// Sort so tests can be deterministic.
		sort.Strings(keys)

		if len(keys) > 1 {
			return fmt.Errorf("step element can only contain a single key, found %d: %s",
				len(keys), strings.Join(keys, ","))
		}
		for stepName := range elem {
			if stepName != RunStepName {
				return fmt.Errorf("%q is not a valid step type", stepName)
			}
		}
		return nil
	}

	if len(s.StringVal) > 0 {
		return validation.Validate(s.StringVal, validation.By(runStep))
	}
	return errors.New("step element is empty")
}

func (s WorkflowHook) ToValid() *valid.WorkflowHook {
	// This will trigger in case #4 (see WorkflowHook docs).
	if len(s.StringVal) > 0 {
		// After validation we assume there's only one key and it's a valid
		// step name so we just use the first one.
		for _, v := range s.StringVal {
			return &valid.WorkflowHook{
				StepName:   RunStepName,
				RunCommand: v,
			}
		}
	}

	panic("step was not valid. This is a bug!")
}

// unmarshalGeneric is used by UnmarshalJSON and UnmarshalYAML to unmarshal
// a step a custom run step: " - run: my custom command"
// It takes a parameter unmarshal that is a function that tries to unmarshal
// the current element into a given object.
func (s *WorkflowHook) unmarshalGeneric(unmarshal func(interface{}) error) error {
	// Try to unmarshal as a custom run step, ex.
	// repo_config:
	// - run: my command
	// We validate if the key is run later.
	var runStep map[string]string
	err := unmarshal(&runStep)
	if err == nil {
		s.StringVal = runStep
		return nil
	}

	return err
}

func (s WorkflowHook) marshalGeneric() (interface{}, error) {
	if len(s.StringVal) != 0 {
		return s.StringVal, nil
	}

	// empty step should be marshalled to null, although this is generally
	// unexpected behavior.
	return nil, nil
}
