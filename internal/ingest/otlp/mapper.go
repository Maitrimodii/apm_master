package otlp

import (
	"fmt"
	"time"

	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/google/uuid"
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

func MapRequest(req IngestRequest, orgID uuid.UUID) (spans []trace.Span, rejected int) {
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

func mapSpan(dto SpanDTO, resourceAttrs map[string]any, orgID uuid.UUID) (trace.Span, error) {
	startTime := time.Unix(0, dto.StartTimeUnixNano).UTC()
	endTime := time.Unix(0, dto.EndTimeUnixNano).UTC()

	if err := validateTime(startTime); err != nil {
		return trace.Span{}, fmt.Errorf("span %s: %w", dto.SpanID, err)
	}
	if err := validateTime(endTime); err != nil {
		return trace.Span{}, fmt.Errorf("span %s: %w", dto.SpanID, err)
	}

	if endTime.Before(startTime) {
		return trace.Span{}, fmt.Errorf("span %s: end_time before start_time", dto.SpanID)
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

	events := make([]trace.SpanEvent, 0, len(dto.Events))
	for _, e := range dto.Events {
		events = append(events, trace.SpanEvent{
			Name:  e.Name,
			Time:  e.TimeUnixNano,
			Attrs: e.Attributes,
		})
	}

	links := make([]trace.SpanLink, 0, len(dto.Links))
	for _, l := range dto.Links {
		links = append(links, trace.SpanLink{
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

	return trace.Span{
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

func validateTime(testRunner time.Time) error {
	now := time.Now()
	// Reject timestamps older than 7 days (retention window) or newer than 24 hours in the future
	if testRunner.Before(now.Add(-7 * 24 * time.Hour)) {
		return fmt.Errorf("timestamp %s is too far in the past (retention limit is 7 days)", testRunner.Format(time.RFC3339))
	}
	if testRunner.After(now.Add(24 * time.Hour)) {
		return fmt.Errorf("timestamp %s is too far in the future (limit is 24 hours)", testRunner.Format(time.RFC3339))
	}
	return nil
}
