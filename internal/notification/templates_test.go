package notification

import (
	"strings"
	"testing"
)

func TestRenderTemplates(t *testing.T) {
	tests := []struct {
		name       string
		eventType  string
		subject    string
		body       string
		metadata   map[string]string
		contains   []string
		unexpected []string
	}{
		{
			name:      "Invite Template",
			eventType: "invite_created",
			subject:   "Invitation",
			body:      "Invitation Body",
			metadata: map[string]string{
				"first_name":  "Alice",
				"org_name":    "Acme Corp",
				"invite_link": "https://example.com/invite/123",
				"expires_at":  "tomorrow",
			},
			contains: []string{
				"Alice",
				"Acme Corp",
				"https://example.com/invite/123",
				"Welcome to APM Master",
			},
		},
		{
			name:      "Password Reset Template",
			eventType: "password_reset_requested",
			subject:   "Reset Request",
			body:      "Reset Body",
			metadata: map[string]string{
				"first_name":      "Bob",
				"reset_link":      "https://example.com/reset/456",
				"expires_in_mins": "30",
			},
			contains: []string{
				"Bob",
				"https://example.com/reset/456",
				"Reset Your Password",
				"30 minutes",
			},
		},
		{
			name:      "Alert Shared Template",
			eventType: "alert_shared",
			subject:   "Alert Shared",
			body:      "Alert Body",
			metadata: map[string]string{
				"alert_name":     "CPU Usage High",
				"severity":       "Critical",
				"condition":      "cpu > 90%",
				"is_enabled":     "true",
				"description":    "Alert when CPU usage exceeds 90%",
				"custom_message": "Please look into this",
			},
			contains: []string{
				"CPU Usage High",
				"badge-critical",
				"cpu &gt; 90%",
				"Active / Enabled",
				"Alert when CPU usage exceeds 90%",
				"Please look into this",
			},
		},
		{
			name:      "Alert Firing Template",
			eventType: "alert_firing",
			subject:   "Alert Firing",
			body:      "Custom body eval details",
			metadata: map[string]string{
				"alert_name": "Memory Usage High",
				"severity":   "Medium",
				"condition":  "mem > 85%",
				"value":      "87.50",
				"threshold":  "85.00",
			},
			contains: []string{
				"Memory Usage High",
				"badge-medium",
				"87.50",
				"85.00",
				"Alert Firing",
			},
		},
		{
			name:      "Generic Fallback Template",
			eventType: "unknown_event_type",
			subject:   "Some Subject",
			body:      "This is a generic message body",
			metadata:  map[string]string{},
			contains: []string{
				"Notification Update",
				"This is a generic message body",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Render(tc.eventType, tc.subject, tc.body, tc.metadata)

			if !strings.Contains(result, "<!DOCTYPE html>") {
				t.Errorf("expected HTML doctype, got raw layout: %s", result)
			}

			for _, expected := range tc.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("expected rendered HTML to contain %q, but it did not.\nRendered:\n%s", expected, result)
				}
			}
		})
	}
}
