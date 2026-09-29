package addonapi

func NewTypedValue(valueType string, value interface{}) map[string]interface{} {
	return map[string]interface{}{"$type": valueType, "value": value}
}

func UnwrapTypedValue(v interface{}) interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		if _, ok := m["$type"].(string); ok {
			if value, exists := m["value"]; exists {
				return value
			}
		}
	}
	return v
}
