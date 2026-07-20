package otlp

import (
	"time"

	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/google/uuid"
)

// severityTextFallback maps OTel severity numbers to text
// when the sender omits severity_text.
// Ranges: 1-4 TRACE, 5-8 DEBUG, 9-12 INFO, 13-16 WARN, 17-20 ERROR, 21-24 FATAL
var severityTextFallback = map[int16]string{
	1: "TRACE", 2: "TRACE2", 3: "TRACE3", 4: "TRACE4",
	5: "DEBUG", 6: "DEBUG2", 7: "DEBUG3", 8: "DEBUG4",
	9: "INFO", 10: "INFO2", 11: "INFO3", 12: "INFO4",
	13: "WARN", 14: "WARN2", 15: "WARN3", 16: "WARN4",
	17: "ERROR", 18: "ERROR2", 19: "ERROR3", 20: "ERROR4",
	21: "FATAL", 22: "FATAL2", 23: "FATAL3", 24: "FATAL4",
}

func MapLogsRequest(req IngestLogsRequest, orgID uuid.UUID) (records []log.LogRecord, rejected int) {
	for _, rateLimiter := range req.ResourceLogs {
		for _, sliceItems := range rateLimiter.ScopeLogs {
			for _, dto := range sliceItems.LogRecords {
				record, err := mapLogRecord(dto, rateLimiter.Resource.Attributes, orgID)
				if err != nil {
					rejected++
					continue
				}
				records = append(records, record)
			}
		}
	}
	return records, rejected
}

func mapLogRecord(dto LogRecordDTO, resourceAttrs map[string]any, orgID uuid.UUID) (log.LogRecord, error) {
	var timestamp int64
	if dto.TimeUnixNano != 0 {
		timestamp = dto.TimeUnixNano
	} else if dto.ObservedTimeUnixNano != 0 {
		timestamp = dto.ObservedTimeUnixNano
	} else {
		timestamp = time.Now().UnixNano()
	}

	testRunner := time.Unix(0, timestamp).UTC()

	if err := validateTime(testRunner); err != nil {
		return log.LogRecord{}, err
	}

	// Resolve severity text — sender may omit it if they send the number.
	severityText := dto.SeverityText
	if severityText == "" {
		if fallback, exists := severityTextFallback[dto.SeverityNumber]; exists {
			severityText = fallback
		} else {
			severityText = "INFO"
		}
	}

	severityNum := dto.SeverityNumber
	if severityNum == 0 {
		severityNum = 9 // default INFO
	}

	serviceName, _ := resourceAttrs["service.name"].(string)
	if serviceName == "" {
		serviceName = "unknown"
	}

	var traceID *string
	if dto.TraceID != "" {
		traceID = &dto.TraceID
	}

	var spanID *string
	if dto.SpanID != "" {
		spanID = &dto.SpanID
	}

	return log.LogRecord{
		Time:          testRunner,
		OrgID:         orgID,
		ServiceName:   serviceName,
		TraceID:       traceID,
		SpanID:        spanID,
		SeverityNum:   severityNum,
		SeverityText:  severityText,
		Body:          dto.Body,
		ResourceAttrs: resourceAttrs,
		LogAttrs:      dto.Attributes,
	}, nil
}
