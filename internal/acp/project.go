package acp

import (
	"encoding/json"
	"errors"
)

var errInvalidSession = errors.New("invalid ACP session response")

// Use raw JSON for the model fields: newer SDK types omit the legacy models
// object, but existing ACP backends may still report it.
func projectSession(raw json.RawMessage, want string) (json.RawMessage, string, error) {
	var response map[string]json.RawMessage
	if json.Unmarshal(raw, &response) != nil {
		return nil, "", errInvalidSession
	}
	var id string
	if json.Unmarshal(response["sessionId"], &id) != nil || id == "" {
		return nil, "", errInvalidSession
	}
	matched := false
	if data, ok := response["models"]; ok && string(data) != "null" {
		var models map[string]json.RawMessage
		var current string
		if json.Unmarshal(data, &models) != nil || json.Unmarshal(models["currentModelId"], &current) != nil || current != want || want == "" {
			return nil, "", ErrModelMismatch
		}
		matched = true
		models["availableModels"], _ = json.Marshal([]map[string]string{{"modelId": want, "name": want}})
		response["models"], _ = json.Marshal(models)
	}
	if data, ok := response["configOptions"]; ok {
		var options []json.RawMessage
		if json.Unmarshal(data, &options) != nil {
			return nil, "", errInvalidSession
		}
		for i, rawOption := range options {
			var fields map[string]json.RawMessage
			if json.Unmarshal(rawOption, &fields) != nil {
				return nil, "", errInvalidSession
			}
			var category, optionID string
			_ = json.Unmarshal(fields["category"], &category)
			_ = json.Unmarshal(fields["id"], &optionID)
			if category != "model" && !(category == "" && optionID == "model") {
				continue
			}
			var current, kind string
			_ = json.Unmarshal(fields["type"], &kind)
			if json.Unmarshal(fields["currentValue"], &current) != nil || current != want || want == "" || kind != "select" {
				return nil, "", ErrModelMismatch
			}
			matched = true
			fields["options"], _ = json.Marshal([]map[string]string{{"value": want, "name": want}})
			options[i], _ = json.Marshal(fields)
		}
		response["configOptions"], _ = json.Marshal(options)
	}
	if !matched {
		return nil, "", ErrModelMismatch
	}
	result, err := json.Marshal(response)
	return result, id, err
}
