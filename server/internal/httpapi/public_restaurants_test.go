package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePublicRestaurantReader struct {
	restaurant publicRestaurant
	err        error
	slug       string
}

func (reader *fakePublicRestaurantReader) FindBySlug(
	_ context.Context,
	slug string,
) (publicRestaurant, error) {
	reader.slug = slug
	return reader.restaurant, reader.err
}

func TestPublicRestaurantEndpointReturnsSafeRestaurantData(t *testing.T) {
	description := "A neighborhood restaurant."
	reader := &fakePublicRestaurantReader{
		restaurant: publicRestaurant{
			Name:              "Sample Restaurant",
			Description:       &description,
			IsActive:          true,
			EnabledQueueTypes: enabledQueueTypes(true, false),
		},
	}
	handler := publicRestaurantTestHandler(reader)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/public/restaurants/sample-restaurant", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if reader.slug != "sample-restaurant" {
		t.Fatalf("slug = %q, want %q", reader.slug, "sample-restaurant")
	}
	for _, expected := range []string{
		`"success":true`,
		`"name":"Sample Restaurant"`,
		`"description":"A neighborhood restaurant."`,
		`"isActive":true`,
		`"enabledQueueTypes":["TABLE"]`,
	} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Errorf("body = %q, want it to contain %s", recorder.Body.String(), expected)
		}
	}
	if strings.Contains(recorder.Body.String(), `"id"`) {
		t.Fatalf("public response exposed an internal ID: %q", recorder.Body.String())
	}
}

func TestPublicRestaurantEndpointReturnsInactiveStateWithConfiguredQueueTypes(t *testing.T) {
	reader := &fakePublicRestaurantReader{
		restaurant: publicRestaurant{
			Name:              "Temporarily Closed",
			IsActive:          false,
			EnabledQueueTypes: enabledQueueTypes(true, true),
		},
	}
	handler := publicRestaurantTestHandler(reader)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/public/restaurants/temporarily-closed", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"isActive":false`) {
		t.Fatalf("body = %q, want inactive restaurant state", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"enabledQueueTypes":["TABLE","PICKUP"]`) {
		t.Fatalf("body = %q, want configured queue types", recorder.Body.String())
	}
}

func TestPublicRestaurantEndpointReturnsNotFoundEnvelope(t *testing.T) {
	handler := publicRestaurantTestHandler(&fakePublicRestaurantReader{err: sql.ErrNoRows})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/public/restaurants/missing", nil),
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"RESTAURANT_NOT_FOUND"`) {
		t.Fatalf("body = %q, want stable restaurant-not-found code", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"data":null`) {
		t.Fatalf("body = %q, want null error data", recorder.Body.String())
	}
}

func TestPublicRestaurantEndpointDoesNotExposeDatabaseErrors(t *testing.T) {
	handler := publicRestaurantTestHandler(&fakePublicRestaurantReader{
		err: errors.New("private database connection detail"),
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/public/restaurants/sample-restaurant", nil),
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "private database connection detail") {
		t.Fatalf("response leaked internal database error: %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("body = %q, want standard internal error code", recorder.Body.String())
	}
}

func TestEnabledQueueTypesPreservePublicOrderAndEmptyArray(t *testing.T) {
	tests := []struct {
		name   string
		table  bool
		pickup bool
		want   []string
	}{
		{name: "none", want: []string{}},
		{name: "table", table: true, want: []string{"TABLE"}},
		{name: "pickup", pickup: true, want: []string{"PICKUP"}},
		{name: "both", table: true, pickup: true, want: []string{"TABLE", "PICKUP"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := enabledQueueTypes(test.table, test.pickup)
			if len(got) != len(test.want) {
				t.Fatalf("types = %#v, want %#v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("types = %#v, want %#v", got, test.want)
				}
			}
		})
	}
}

func publicRestaurantTestHandler(reader publicRestaurantReader) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	registerPublicRestaurantRoutes(mux, reader, logger)
	return mux
}
