package serializer

import (
	"encoding/json"
	"fmt"

	"github.com/Maitrimodii/apm_master/internal/ingest/model"
)

type JSON struct{}

func (JSON) Marshal(event model.Event) ([]byte, error) {
	if event.SchemaVersion == 0 {
		event.SchemaVersion = model.SchemaVersion
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal telemetry event: %w", err)
	}
	return payload, nil
}

func (JSON) Unmarshal(payload []byte) (model.Event, error) {
	var event model.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return model.Event{}, fmt.Errorf("unmarshal telemetry event: %w", err)
	}
	if event.SchemaVersion != model.SchemaVersion {
		return model.Event{}, fmt.Errorf("unsupported telemetry schema version %d", event.SchemaVersion)
	}
	if event.Signal == "" {
		return model.Event{}, fmt.Errorf("telemetry event signal is required")
	}
	if len(event.Payload) == 0 {
		return model.Event{}, fmt.Errorf("telemetry event payload is required")
	}
	return event, nil
}

func MarshalPayload[T any](payload T) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal telemetry payload: %w", err)
	}
	return data, nil
}

func UnmarshalPayload[T any](data []byte) (T, error) {
	var payload T
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf("unmarshal telemetry payload: %w", err)
	}
	return payload, nil
}
