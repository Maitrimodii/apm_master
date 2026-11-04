package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newTestContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}

func TestParsePage(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantNumber int
		wantSize   int
		wantOffset int
		wantErr    bool
	}{
		{name: "defaults", target: "/x", wantNumber: 1, wantSize: 25},
		{name: "explicit", target: "/x?page=3&limit=10", wantNumber: 3, wantSize: 10, wantOffset: 20},
		{name: "zero page", target: "/x?page=0", wantErr: true},
		{name: "negative limit", target: "/x?limit=-1", wantErr: true},
		{name: "limit above max", target: "/x?limit=1001", wantErr: true},
		{name: "non-numeric", target: "/x?page=abc", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := ParsePage(newTestContext(tc.target), 25)
			if tc.wantErr {
				if err == nil {
					t.Fatal("ParsePage() error = nil, want validation error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePage() error = %v", err)
			}
			if page.Number != tc.wantNumber || page.Size != tc.wantSize || page.Offset() != tc.wantOffset {
				t.Errorf("ParsePage() = (%d, %d, offset %d), want (%d, %d, offset %d)",
					page.Number, page.Size, page.Offset(), tc.wantNumber, tc.wantSize, tc.wantOffset)
			}
		})
	}
}

func TestParseTimeRange(t *testing.T) {
	tr, err := ParseTimeRange(newTestContext("/x?from=2026-01-01T10:00:00Z&to=2026-01-01T11:00:00Z"))
	if err != nil {
		t.Fatalf("ParseTimeRange() error = %v", err)
	}
	if want := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC); !tr.From.Equal(want) {
		t.Errorf("From = %v, want %v", tr.From, want)
	}

	tr, err = ParseTimeRange(newTestContext("/x"))
	if err != nil {
		t.Fatalf("ParseTimeRange() default error = %v", err)
	}
	if got := tr.To.Sub(tr.From); got != DefaultLookback {
		t.Errorf("default window = %v, want %v", got, DefaultLookback)
	}

	for _, target := range []string{"/x?from=yesterday", "/x?from=2026-01-01T11:00:00Z&to=2026-01-01T10:00:00Z"} {
		if _, err := ParseTimeRange(newTestContext(target)); err == nil {
			t.Errorf("ParseTimeRange(%q) error = nil, want validation error", target)
		}
	}
}

func TestOrgID(t *testing.T) {
	c := newTestContext("/x")
	if _, err := OrgID(c); err == nil {
		t.Fatal("OrgID() error = nil for missing org_id")
	}
	want := uuid.New()
	c.Set(CtxKeyOrgID, want.String())
	got, err := OrgID(c)
	if err != nil || got != want {
		t.Fatalf("OrgID() = (%v, %v), want (%v, nil)", got, err, want)
	}
}

func TestBearerToken(t *testing.T) {
	c := newTestContext("/x")
	c.Request.Header.Set(HeaderAuthorization, "Bearer abc")
	if tok, err := BearerToken(c); err != nil || tok != "abc" {
		t.Fatalf("BearerToken() = (%q, %v), want (abc, nil)", tok, err)
	}
	for _, header := range []string{"", "abc", "Basic abc", "Bearer "} {
		c.Request.Header.Set(HeaderAuthorization, header)
		if _, err := BearerToken(c); err == nil {
			t.Errorf("BearerToken(%q) error = nil, want error", header)
		}
	}
}
