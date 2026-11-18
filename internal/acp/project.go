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
	matched, err := projectModels(response, want, true)
	if err != nil {
		return nil, "", err
	}
	if !matched {
		return nil, "", ErrModelMismatch
	}
	result, err := json.Marshal(response)
	return result, id, err
}

// Dynamic projections preserve the actual current model in observe mode,
// but never advertise additional selectable models.
func projectModels(response map[string]json.RawMessage, want string, strict bool) (bool, error) {
	matched := false
	if data, ok := response["models"]; ok && string(data) != "null" {
		var models map[string]json.RawMessage
		var current string
		if json.Unmarshal(data, &models) != nil || json.Unmarshal(models["currentModelId"], &current) != nil || (strict && current != want) || current == "" || want == "" {
			return false, ErrModelMismatch
		}
		matched = true
		models["availableModels"], _ = json.Marshal([]map[string]string{{"modelId": want, "name": want}})
		response["models"], _ = json.Marshal(models)
	}
	if data, ok := response["configOptions"]; ok {
		var options []json.RawMessage
		if json.Unmarshal(data, &options) != nil {
			return false, errInvalidSession
		}
		for i, rawOption := range options {
			var fields map[string]json.RawMessage
			if json.Unmarshal(rawOption, &fields) != nil {
				return false, errInvalidSession
			}
			var category, optionID string
			_ = json.Unmarshal(fields["category"], &category)
			_ = json.Unmarshal(fields["id"], &optionID)
			if category != "model" && !(category == "" && optionID == "model") {
				continue
			}
			var current, kind string
			_ = json.Unmarshal(fields["type"], &kind)
			if json.Unmarshal(fields["currentValue"], &current) != nil || (strict && current != want) || current == "" || want == "" || kind != "select" {
				return false, ErrModelMismatch
			}
			matched = true
			fields["options"], _ = json.Marshal([]map[string]string{{"value": want, "name": want}})
			options[i], _ = json.Marshal(fields)
		}
		response["configOptions"], _ = json.Marshal(options)
	}
	return matched, nil
}

func projectConfigResponse(raw json.RawMessage, want string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errInvalidSession
	}
	if _, err := projectModels(fields, want, false); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func projectUpdate(raw json.RawMessage, want string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	var update map[string]json.RawMessage
	var kind string
	if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(fields["update"], &update) != nil || json.Unmarshal(update["sessionUpdate"], &kind) != nil {
		return nil, errInvalidSession
	}
	if kind != "config_option_update" && kind != "model_update" {
		return raw, nil
	}
	projected, err := projectConfigResponse(fields["update"], want)
	if err != nil {
		return nil, err
	}
	fields["update"] = projected
	return json.Marshal(fields)
}
