package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/nat-link/nat-link/internal/model"
)

func TestLogQuerySearchesBeyondFirstPage(t *testing.T) {
	store, _, router, session, _ := managementRouter(t)
	if err := store.RecordEvent(context.Background(), "warn", "old.target", "needle-oldest", nil); err != nil {
		t.Fatal(err)
	}
	for range 120 {
		if err := store.RecordEvent(context.Background(), "info", "recent", "ordinary", nil); err != nil {
			t.Fatal(err)
		}
	}
	response := requestJSON(t, router, http.MethodGet, "/api/logs?keyword=needle-oldest", session, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("query status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []model.Event `json:"items"`
		Total int64         `json:"total"`
	}
	decodeJSON(t, response, &payload)
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].Message != "needle-oldest" {
		t.Fatalf("query did not search all logs: %+v", payload)
	}
}
