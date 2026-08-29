package events

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewEvent(t *testing.T) {
	taskID := uuid.New()
	payload := map[string]interface{}{
		"key": "value",
		"nested": map[string]interface{}{
			"inner": 42,
		},
	}

	before := time.Now().UTC()
	event := NewEvent(taskID, TaskStarted, payload)
	after := time.Now().UTC()

	if event.ID == uuid.Nil {
		t.Error("expected valid event ID, got nil UUID")
	}
	if event.TaskID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, event.TaskID)
	}
	if event.Type != TaskStarted {
		t.Errorf("expected type %s, got %s", TaskStarted, event.Type)
	}
	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Errorf("expected timestamp between %v and %v, got %v", before, after, event.Timestamp)
	}

	// Modify original payload, verify event is unaffected
	payload["key"] = "changed"
	payload["nested"].(map[string]interface{})["inner"] = 99

	if event.Payload["key"] != "value" {
		t.Errorf("expected event payload key to be 'value', got %v", event.Payload["key"])
	}
	nested := event.Payload["nested"].(map[string]interface{})
	if nested["inner"] != 42 {
		t.Errorf("expected event payload nested inner to be 42, got %v", nested["inner"])
	}
}

func TestEventClone(t *testing.T) {
	event := Event{
		ID:        uuid.New(),
		TaskID:    uuid.New(),
		Type:      FileChanged,
		Timestamp: time.Now().UTC(),
		Payload: map[string]interface{}{
			"list": []string{"a", "b"},
			"dict": map[string]interface{}{"c": "d"},
		},
	}

	cloned := event.Clone()

	if cloned.ID != event.ID || cloned.TaskID != event.TaskID || cloned.Type != event.Type || !cloned.Timestamp.Equal(event.Timestamp) {
		t.Error("basic fields were not cloned correctly")
	}

	// Mutate original
	event.Payload["list"].([]string)[0] = "x"
	event.Payload["dict"].(map[string]interface{})["c"] = "y"
	delete(event.Payload, "list")

	// Verify clone is isolated
	if cloned.Payload["list"] == nil {
		t.Fatal("cloned payload list should not be nil")
	}
	list := cloned.Payload["list"].([]string)
	if list[0] != "a" {
		t.Errorf("expected cloned list[0] to be 'a', got %s", list[0])
	}
	dict := cloned.Payload["dict"].(map[string]interface{})
	if dict["c"] != "d" {
		t.Errorf("expected cloned dict['c'] to be 'd', got %s", dict["c"])
	}
}

func TestClonePayload(t *testing.T) {
	if ClonePayload(nil) != nil {
		t.Error("expected nil clone for nil input")
	}

	original := map[string]interface{}{
		"str": "hello",
		"int": 10,
		"nested_map": map[string]interface{}{
			"bool": true,
		},
		"slice_iface": []interface{}{"a", 1},
		"slice_str":   []string{"x", "y"},
	}

	cloned := ClonePayload(original)

	// Mutate original
	original["str"] = "world"
	original["nested_map"].(map[string]interface{})["bool"] = false
	original["slice_iface"].([]interface{})[0] = "b"
	original["slice_str"].([]string)[0] = "z"

	// Verify clone
	if cloned["str"] != "hello" {
		t.Errorf("expected 'hello', got %v", cloned["str"])
	}
	if cloned["nested_map"].(map[string]interface{})["bool"] != true {
		t.Errorf("expected true, got %v", cloned["nested_map"].(map[string]interface{})["bool"])
	}
	if cloned["slice_iface"].([]interface{})[0] != "a" {
		t.Errorf("expected 'a', got %v", cloned["slice_iface"].([]interface{})[0])
	}
	if cloned["slice_str"].([]string)[0] != "x" {
		t.Errorf("expected 'x', got %v", cloned["slice_str"].([]string)[0])
	}
}

func TestCloneValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{"nil", nil, nil},
		{"string", "abc", "abc"},
		{"int", 123, 123},
		{"float", 3.14, 3.14},
		{"nested_map", map[string]interface{}{"k": "v"}, map[string]interface{}{"k": "v"}},
		{"slice_iface", []interface{}{1, "a"}, []interface{}{1, "a"}},
		{"slice_string", []string{"a", "b"}, []string{"a", "b"}},
		{"unknown_type", struct{ A int }{1}, struct{ A int }{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloneValue(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("cloneValue() = %v, want %v", got, tt.expected)
			}

			// For reference types, verify it's a new instance by modifying the original if possible
			switch orig := tt.input.(type) {
			case map[string]interface{}:
				orig["k"] = "changed"
				if got.(map[string]interface{})["k"] != "v" {
					t.Error("map was not deep copied")
				}
			case []interface{}:
				orig[0] = 999
				if got.([]interface{})[0] != 1 {
					t.Error("[]interface{} was not deep copied")
				}
			case []string:
				orig[0] = "changed"
				if got.([]string)[0] != "a" {
					t.Error("[]string was not deep copied")
				}
			}
		})
	}
}
