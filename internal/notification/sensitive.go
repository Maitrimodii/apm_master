package notification

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Sensitive notifications carry a one-time link (a password reset or an
// invite) that grants access to whoever holds it. Their body and metadata are
// stored only encrypted (see Store.CreateJob) and never reach
// notification_events: the event row gets a placeholder body and metadata
// without the link keys. Migration 000015 adds CHECK constraints that reject
// rows breaking either rule, so even an outdated binary cannot store them.

// sensitiveEventTypes are the event types whose emails carry one-time links.
var sensitiveEventTypes = map[EventType]bool{
	EventInviteCreated:          true,
	EventPasswordResetRequested: true,
}

// secretMetadataKeys are metadata keys whose values are one-time links.
var secretMetadataKeys = []string{"invite_link", "reset_link"}

const (
	// encryptedBody stands in for the body of a sensitive job; the real body
	// is in payload_ciphertext.
	encryptedBody = "[encrypted]"
	// redactedBody stands in for the body of a sensitive event record.
	redactedBody = "[redacted]"
)

// IsSensitive reports whether emails of this event type carry one-time links.
func IsSensitive(eventType EventType) bool { return sensitiveEventTypes[eventType] }

// publicMetadata returns metadata without secret keys.
func publicMetadata(metadata map[string]string) map[string]string {
	out := make(map[string]string, len(metadata))
	for k, v := range metadata {
		out[k] = v
	}
	for _, k := range secretMetadataKeys {
		delete(out, k)
	}
	return out
}

func hasSecretMetadata(metadata map[string]string) bool {
	for _, k := range secretMetadataKeys {
		if _, ok := metadata[k]; ok {
			return true
		}
	}
	return false
}

// sealedPayload is what a sensitive job encrypts: everything the email
// template needs, including the link.
type sealedPayload struct {
	Body     string            `json:"body"`
	Metadata map[string]string `json:"metadata"`
}

// payloadAAD binds a job's ciphertext to the job it belongs to. Associated
// data is authenticated, not encrypted: ciphertext copied to another row
// (different ID, org, event type or recipient) fails to decrypt.
func payloadAAD(jobID, orgID uuid.UUID, eventType EventType, recipient string) []byte {
	return fmt.Appendf(nil, "apm.notification_job.v1\x00%s\x00%s\x00%s\x00%s", jobID, orgID, eventType, recipient)
}

func encodePayload(body string, metadata map[string]string) ([]byte, error) {
	return json.Marshal(sealedPayload{Body: body, Metadata: metadata})
}

func decodePayload(raw []byte) (sealedPayload, error) {
	var p sealedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return sealedPayload{}, fmt.Errorf("decode notification payload: %w", err)
	}
	return p, nil
}
