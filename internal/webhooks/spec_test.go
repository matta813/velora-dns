package webhooks

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// The documented event list must match what the server accepts.
func TestEventTypesMatchOpenAPI(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Items struct {
					Enum []string `json:"enum"`
				} `json:"items"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err = json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if got := spec.Components.Schemas["WebhookEventTypes"].Items.Enum; !slices.Equal(got, EventTypes) {
		t.Fatalf("openapi %v, server %v", got, EventTypes)
	}
}
