package state

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// generateShortID creates a random 8-character hex string
func generateShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Safe payload extraction helpers

func getString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			if s, ok := val.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func getStringSlice(m map[string]interface{}, keys ...string) []string {
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			if slice, ok := val.([]string); ok {
				return slice
			}
			if slice, ok := val.([]interface{}); ok {
				res := make([]string, 0, len(slice))
				for _, item := range slice {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						res = append(res, strings.TrimSpace(s))
					}
				}
				return res
			}
		}
	}
	return nil
}

func getInt(m map[string]interface{}, keys ...string) (int, bool) {
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			switch v := val.(type) {
			case int:
				return v, true
			case int64:
				return int(v), true
			case float64:
				return int(v), true
			}
		}
	}
	return 0, false
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

func removeString(slice []string, target string) []string {
	res := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != target {
			res = append(res, s)
		}
	}
	return res
}
