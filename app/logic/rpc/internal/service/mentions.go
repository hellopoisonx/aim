package service

import "encoding/json"

// ParseMentionsJSON unmarshals mentions JSONB (array of decimal user id strings).
// Invalid or empty input returns nil.
func ParseMentionsJSON(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var mentions []string
	if err := json.Unmarshal(data, &mentions); err != nil {
		return nil
	}
	return mentions
}
