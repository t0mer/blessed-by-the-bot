package provider

import (
	"fmt"
	"strings"
)

// Chat-id suffixes used by WhatsApp.
const (
	privateSuffix = "@c.us"
	groupSuffix   = "@g.us"
)

// E.164 allows 8 to 15 digits including the country code.
const (
	minPhoneDigits = 8
	maxPhoneDigits = 15
)

// NormalizeChatID turns a stored phone number into a provider chat id.
//
// Values that already look like a JID (they contain "@") are returned unchanged,
// so group ids such as 120363…@g.us survive untouched. Everything else is
// stripped to digits and given the private-chat suffix.
func NormalizeChatID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("chat id is empty")
	}
	if strings.Contains(trimmed, "@") {
		if strings.HasPrefix(trimmed, "@") {
			return "", fmt.Errorf("chat id %q has no local part", value)
		}
		return trimmed, nil
	}

	var digits strings.Builder
	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	phone := digits.String()
	if len(phone) < minPhoneDigits || len(phone) > maxPhoneDigits {
		return "", fmt.Errorf("phone number %q has %d digits, want %d-%d",
			value, len(phone), minPhoneDigits, maxPhoneDigits)
	}
	return phone + privateSuffix, nil
}

// IsGroupChatID reports whether a chat id addresses a group.
func IsGroupChatID(chatID string) bool {
	return strings.HasSuffix(chatID, groupSuffix)
}
