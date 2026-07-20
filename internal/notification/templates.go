package notification

import (
	"bytes"
	"html/template"
	"strings"
)

// EmailTemplateData contains all structured fields needed for template rendering
type EmailTemplateData struct {
	Subject       string
	Body          string
	EventType     string
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

const emailBaseTemplate = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Subject}}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background-color: #f4f5f7;
            color: #1e293b;
            margin: 0;
            padding: 0;
            line-height: 1.6;
        }
        .container {
            max-width: 600px;
            margin: 40px auto;
            background: #ffffff;
            border-radius: 12px;
            overflow: hidden;
            box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -1px rgba(0, 0, 0, 0.03);
            border: 1px solid #e2e8f0;
        }
        .top-bar {
            height: 6px;
            background-color: {{.HeaderBg}};
        }
        .header {
            padding: 32px 32px 24px;
            text-align: left;
            border-bottom: 1px solid #f1f5f9;
        }
        .brand {
            font-size: 20px;
            font-weight: 700;
            color: #0f172a;
            letter-spacing: -0.025em;
            display: flex;
            align-items: center;
        }
        .brand-accent {
            color: #4f46e5;
            margin-right: 4px;
        }
        .content {
            padding: 32px;
        }
        h1 {
            font-size: 22px;
            font-weight: 700;
            color: #0f172a;
            margin-top: 0;
            margin-bottom: 16px;
        }
        p {
            margin-top: 0;
            margin-bottom: 20px;
            color: #475569;
            font-size: 16px;
        }
        .btn-container {
            margin: 32px 0;
        }
        .btn {
            display: inline-block;
            background-color: #4f46e5;
            color: #ffffff !important;
            text-decoration: none;
            padding: 12px 24px;
            font-weight: 600;
            border-radius: 6px;
            font-size: 15px;
            text-align: center;
            box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06);
            transition: background-color 0.2s;
        }
        .btn:hover {
            background-color: #4338ca;
        }
        .details-card {
            background-color: #f8fafc;
            border: 1px solid #e2e8f0;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 24px;
        }
        .details-title {
            font-weight: 700;
            color: #334155;
            margin-bottom: 12px;
            font-size: 14px;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        .details-grid {
            display: table;
            width: 100%;
        }
        .details-row {
            display: table-row;
        }
        .details-label {
            display: table-cell;
            font-weight: 600;
            color: #64748b;
            padding: 6px 12px 6px 0;
            font-size: 14px;
            width: 35%;
        }
        .details-value {
            display: table-cell;
            color: #0f172a;
            padding: 6px 0;
            font-size: 14px;
        }
        .badge {
            display: inline-block;
            padding: 2px 8px;
            font-size: 12px;
            font-weight: 600;
            border-radius: 9999px;
            text-transform: uppercase;
        }
        .badge-critical { background-color: #fee2e2; color: #991b1b; }
        .badge-high { background-color: #ffedd5; color: #9a3412; }
        .badge-medium { background-color: #fef9c3; color: #854d0e; }
        .badge-low { background-color: #f0fdf4; color: #166534; }
        .badge-resolved { background-color: #dcfce7; color: #166534; }
        .badge-acknowledged { background-color: #dbeafe; color: #1e40af; }
        
        .footer {
            padding: 24px 32px 40px;
            text-align: center;
            font-size: 13px;
            color: #94a3b8;
            border-top: 1px solid #f1f5f9;
        }
        .footer a {
            color: #4f46e5;
            text-decoration: none;
        }
        .muted-text {
            font-size: 13px;
            color: #94a3b8;
        }
        .message-quote {
            border-left: 4px solid #cbd5e1;
            padding-left: 16px;
            margin: 16px 0;
            font-style: italic;
            color: #475569;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="top-bar"></div>
        <div class="header">
            <div class="brand">
                <span class="brand-accent">▲</span> APM Master
            </div>
        </div>
        <div class="content">
            {{if eq .EventType "invite_created"}}
                <h1>Welcome to APM Master</h1>
                <p>Hi {{.FirstName}},</p>
                <p>You have been invited to join <strong>{{.OrgName}}</strong> on APM Master, our application performance monitoring platform.</p>
                
                <div class="btn-container">
                    <a href="{{.InviteLink}}" class="btn">Accept Invitation</a>
                </div>
                
                <p class="muted-text">This invitation was sent to your registered email and will expire on {{.ExpiresAt}}.</p>
            
            {{else if eq .EventType "password_reset_requested"}}
                <h1>Reset Your Password</h1>
                <p>Hi {{.FirstName}},</p>
                <p>We received a request to reset your APM Master password. Click the button below to secure your account and choose a new password.</p>
                
                <div class="btn-container">
                    <a href="{{.ResetLink}}" class="btn">Reset Password</a>
                </div>
                
                <p>If you didn't request this change, you can safely ignore this email.</p>
                <p class="muted-text">This link will expire in {{.ExpiresInMins}} minutes.</p>

            {{else if eq .EventType "alert_shared"}}
                <h1>Alert Shared with You</h1>
                <p>An alert rule from APM Master has been shared with you.</p>
                
                <div class="details-card">
                    <div class="details-title">Alert Rules Details</div>
                    <div class="details-grid">
                        <div class="details-row">
                            <div class="details-label">Alert Name</div>
                            <div class="details-value"><strong>{{.AlertName}}</strong></div>
                        </div>
                        <div class="details-row">
                            <div class="details-label">Severity</div>
                            <div class="details-value"><span class="badge badge-{{html (lower .Severity)}}">{{.Severity}}</span></div>
                        </div>
                        <div class="details-row">
                            <div class="details-label">Condition</div>
                            <div class="details-value"><code>{{.Condition}}</code></div>
                        </div>
                        <div class="details-row">
                            <div class="details-label">Status</div>
                            <div class="details-value">{{if eq .IsEnabled "true"}}Active / Enabled{{else}}Disabled{{end}}</div>
                        </div>
                        {{if .Description}}
                        <div class="details-row">
                            <div class="details-label">Description</div>
                            <div class="details-value">{{.Description}}</div>
                        </div>
                        {{end}}
                    </div>
                </div>

                {{if .CustomMessage}}
                <p><strong>Note:</strong></p>
                <div class="message-quote">
                    {{.CustomMessage}}
                </div>
                {{end}}


                {{if .Body}}
                <div class="message-quote">
                    {{.Body}}
                </div>
                {{end}}

            {{else}}
                <!-- Generic Fallback Template -->
                <h1>Notification Update</h1>
                <p>{{.Body}}</p>
            {{end}}
        </div>
        <div class="footer">
            <p>© {{.OrgName}} APM Master. All rights reserved.</p>
            <p class="muted-text">You are receiving this system notification because you are a registered user of APM Master.</p>
        </div>
    </div>
</body>
</html>
`

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
func Render(eventType string, subject string, body string, metadata map[string]string) string {
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
	case "alert_firing":
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
	case "alert_resolved":
		data.HeaderBg = "#10b981" // Emerald green
	case "alert_acknowledged":
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
