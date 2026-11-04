package otlp

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/traces"
)

var spanKindString = map[int]string{
	0: "UNSPECIFIED",
	1: "INTERNAL",
	2: "SERVER",
	3: "CLIENT",
	4: "PRODUCER",
	5: "CONSUMER",
}

var statusCodeString = map[int]string{
	0: "UNSET",
	1: "OK",
	2: "ERROR",
}

func MapRequest(req IngestRequest, orgID uuid.UUID) (spans []traces.Span, rejected int) {
	for _, resultSet := range req.ResourceSpans {
		for _, stringSlice := range resultSet.ScopeSpans {
			for _, dto := range stringSlice.Spans {
				span, err := mapSpan(dto, resultSet.Resource.Attributes, orgID)
				if err != nil {
					rejected++
					continue
				}
				spans = append(spans, span)
			}
		}
	}
	return spans, rejected
}

func mapSpan(dto SpanDTO, resourceAttrs map[string]any, orgID uuid.UUID) (traces.Span, error) {
	startTime := time.Unix(0, dto.StartTimeUnixNano).UTC()
	endTime := time.Unix(0, dto.EndTimeUnixNano).UTC()

	if err := validateTime(startTime); err != nil {
		return traces.Span{}, fmt.Errorf("span %s: %w", dto.SpanID, err)
	}
	if err := validateTime(endTime); err != nil {
		return traces.Span{}, fmt.Errorf("span %s: %w", dto.SpanID, err)
	}

	if endTime.Before(startTime) {
		return traces.Span{}, fmt.Errorf("span %s: end_time before start_time", dto.SpanID)
	}

	durationNS := dto.EndTimeUnixNano - dto.StartTimeUnixNano

	kind, exists := spanKindString[dto.Kind]
	if !exists {
		kind = "UNSPECIFIED"
	}

	status, exists := statusCodeString[dto.Status.Code]
	if !exists {
		status = "UNSET"
	}

	var parentID *string
	if dto.ParentSpanID != "" {
		parentID = &dto.ParentSpanID
	}

	var statusMsg *string
	if dto.Status.Message != "" {
		statusMsg = &dto.Status.Message
	}

	events := make([]traces.SpanEvent, 0, len(dto.Events))
	for _, e := range dto.Events {
		events = append(events, traces.SpanEvent{
			Name:  e.Name,
			Time:  e.TimeUnixNano,
			Attrs: e.Attributes,
		})
	}

	links := make([]traces.SpanLink, 0, len(dto.Links))
	for _, l := range dto.Links {
		links = append(links, traces.SpanLink{
			TraceID:    l.TraceID,
			SpanID:     l.SpanID,
			TraceState: l.TraceState,
			Attrs:      l.Attributes,
		})
	}

	serviceName, _ := resourceAttrs["service.name"].(string)
	if serviceName == "" {
		serviceName = "unknown"
	}

	return traces.Span{
		SpanID:        dto.SpanID,
		TraceID:       dto.TraceID,
		ParentID:      parentID,
		OrgID:         orgID,
		ServiceName:   serviceName,
		SpanName:      dto.Name,
		SpanKind:      kind,
		StartTime:     startTime,
		EndTime:       endTime,
		DurationNS:    durationNS, // computed from wire values, stored as-is
		StatusCode:    status,
		StatusMessage: statusMsg,
		ResourceAttrs: resourceAttrs,
		SpanAttrs:     dto.Attributes,
		Events:        events,
		Links:         links,
	}, nil
}

func validateTime(ts time.Time) error {
	now := time.Now()
	// Reject timestamps older than 7 days (retention window) or newer than 24 hours in the future
	if ts.Before(now.Add(-7 * 24 * time.Hour)) {
		return fmt.Errorf("timestamp %s is too far in the past (retention limit is 7 days)", ts.Format(time.RFC3339))
	}
	if ts.After(now.Add(24 * time.Hour)) {
		return fmt.Errorf("timestamp %s is too far in the future (limit is 24 hours)", ts.Format(time.RFC3339))
	}
	return nil
}
