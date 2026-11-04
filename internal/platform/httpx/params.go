package httpx

import (
	"strconv"
	"strings"
	"time"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Paging and time-range defaults shared by every list endpoint.
const (
	DefaultPageSize = 50
	MaxPageSize     = 1000
	DefaultLookback = time.Hour
)

// Page is a parsed ?page=&limit= pair.
type Page struct {
	Number int
	Size   int
}

// Offset is the number of rows to skip for this page.
func (p Page) Offset() int { return (p.Number - 1) * p.Size }

// ParsePage reads ?page= (default 1) and ?limit= (default defaultSize, at most
// MaxPageSize). Non-numeric or out-of-range values are a 400.
func ParsePage(c *gin.Context, defaultSize int) (Page, *apperr.Error) {
	number, err := queryInt(c, "page", 1)
	if err != nil || number < 1 {
		return Page{}, apperr.ValidationError("page", "must be a positive integer")
	}
	size, err := queryInt(c, "limit", defaultSize)
	if err != nil || size < 1 || size > MaxPageSize {
		return Page{}, apperr.ValidationError("limit", "must be between 1 and "+strconv.Itoa(MaxPageSize))
	}
	return Page{Number: number, Size: size}, nil
}

// TimeRange is a parsed ?from=&to= window.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// ParseTimeRange reads RFC3339 ?from= and ?to=, defaulting to the last
// DefaultLookback. A malformed timestamp or to < from is a 400.
func ParseTimeRange(c *gin.Context) (TimeRange, *apperr.Error) {
	now := time.Now().UTC()
	tr := TimeRange{From: now.Add(-DefaultLookback), To: now}
	if v := c.Query("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return TimeRange{}, apperr.ValidationError("from", "must be an RFC3339 timestamp")
		}
		tr.From = t
	}
	if v := c.Query("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return TimeRange{}, apperr.ValidationError("to", "must be an RFC3339 timestamp")
		}
		tr.To = t
	}
	if tr.To.Before(tr.From) {
		return TimeRange{}, apperr.ValidationError("to", "must be after from")
	}
	return tr, nil
}

// QueryFloat reads an optional float query parameter; absent means 0.
func QueryFloat(c *gin.Context, name string) (float64, *apperr.Error) {
	v := c.Query(name)
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, apperr.ValidationError(name, "must be a number")
	}
	return f, nil
}

// QueryInt reads an optional integer query parameter with a default.
func QueryInt(c *gin.Context, name string, def int) (int, *apperr.Error) {
	n, err := queryInt(c, name, def)
	if err != nil {
		return 0, apperr.ValidationError(name, "must be an integer")
	}
	return n, nil
}

// PathUUID parses a UUID path parameter.
func PathUUID(c *gin.Context, name string) (uuid.UUID, *apperr.Error) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, apperr.ValidationError(name, "must be a UUID")
	}
	return id, nil
}

func queryInt(c *gin.Context, name string, def int) (int, error) {
	v := strings.TrimSpace(c.Query(name))
	if v == "" {
		return def, nil
	}
	return strconv.Atoi(v)
}
