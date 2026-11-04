package notification

import (
	"bytes"
	_ "embed"
	"html/template"
	"strings"
)

// EmailTemplateData contains all structured fields needed for template rendering
type EmailTemplateData struct {
	Subject       string
	Body          string
	EventType     EventType
	OrgName       string
	FirstName     string
	InviteLink    string
	ResetLink     string
	ExpiresAt     string
	ExpiresInMins string
	AlertName     string
	Severity      string
	Condition     string
	IsEnabled     string
	Description   string
	CustomMessage string
	Value         string
	Threshold     string
	State         string
	BrandColor    string
	HeaderBg      string
}

//go:embed templates/email.html
var emailBaseTemplate string

var (
	tmpl *template.Template
)

func init() {
	funcMap := template.FuncMap{
		"lower": strings.ToLower,
	}
	tmpl = template.Must(template.New("email").Funcs(funcMap).Parse(emailBaseTemplate))
}

// Render compiles the HTML email layout based on event type and metadata
func Render(eventType EventType, subject string, body string, metadata map[string]string) string {
	data := EmailTemplateData{
		Subject:       subject,
		Body:          body,
		EventType:     eventType,
		OrgName:       "APM Master", // default fallback
		FirstName:     "User",       // default fallback
		InviteLink:    metadata["invite_link"],
		ResetLink:     metadata["reset_link"],
		ExpiresAt:     metadata["expires_at"],
		ExpiresInMins: metadata["expires_in_mins"],
		AlertName:     metadata["alert_name"],
		Severity:      metadata["severity"],
		Condition:     metadata["condition"],
		IsEnabled:     metadata["is_enabled"],
		Description:   metadata["description"],
		CustomMessage: metadata["custom_message"],
		Value:         metadata["value"],
		Threshold:     metadata["threshold"],
		State:         metadata["state"],
	}

	if metadata["first_name"] != "" {
		data.FirstName = metadata["first_name"]
	}
	if metadata["org_name"] != "" {
		data.OrgName = metadata["org_name"]
	}
	if data.Severity == "" {
		data.Severity = "CRITICAL"
	}

	// Select colors based on EventType/Severity
	data.BrandColor = "#4f46e5" // Default brand purple
	switch eventType {
	case EventAlertFiring:
		switch strings.ToLower(data.Severity) {
		case "low":
			data.HeaderBg = "#f59e0b" // Amber
		case "medium":
			data.HeaderBg = "#f97316" // Orange
		case "high", "critical":
			data.HeaderBg = "#ef4444" // Red
		default:
			data.HeaderBg = "#e11d48" // Rose
		}
	case EventAlertResolved:
		data.HeaderBg = "#10b981" // Emerald green
	case EventAlertAcknowledged:
		data.HeaderBg = "#3b82f6" // Blue
	default:
		data.HeaderBg = "#4f46e5" // Brand Purple
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// Fallback to plain body wrapped in simple structure
		return `<html><body>` + body + `</body></html>`
	}
	return buf.String()
}
