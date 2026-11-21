package completiongates

import "encoding/json"

// payloadMap decodes a bus payload into a generic map, whatever its type.
func payloadMap(data any) map[string]any {
	if payload, ok := data.(map[string]any); ok {
		return payload
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var payload map[string]any
	if json.Unmarshal(encoded, &payload) != nil {
		return nil
	}
	return payload
}
