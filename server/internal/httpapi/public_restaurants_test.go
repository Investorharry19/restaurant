package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
)

type fakePublicRestaurantReader struct {
	restaurant publicRestaurant
	logo       []byte
	logoType   string
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

func (reader *fakePublicRestaurantReader) FindLogoBySlug(
	_ context.Context,
	slug string,
) ([]byte, string, error) {
	reader.slug = slug
	if reader.logo == nil {
		return nil, "", sql.ErrNoRows
	}
	return reader.logo, reader.logoType, reader.err
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

func TestPublicRestaurantLogoEndpointReturnsStoredImageWithSafeHeaders(t *testing.T) {
	logoData := []byte("stored-logo-bytes")
	reader := &fakePublicRestaurantReader{
		logo:     logoData,
		logoType: "image/png",
	}
	handler := publicRestaurantTestHandler(reader)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/public/restaurants/sample-restaurant/logo", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", recorder.Header().Get("X-Content-Type-Options"))
	}
	if !bytes.Equal(recorder.Body.Bytes(), logoData) {
		t.Fatalf("logo body = %q, want stored bytes %q", recorder.Body.Bytes(), logoData)
	}
	if reader.slug != "sample-restaurant" {
		t.Fatalf("slug = %q, want sample-restaurant", reader.slug)
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

func TestDecodeRestaurantLogoAcceptsBoundedPNGAndRejectsMismatches(t *testing.T) {
	var encodedPNG bytes.Buffer
	source := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			source.Set(x, y, color.RGBA{R: 220, A: 255})
		}
	}
	if err := png.Encode(&encodedPNG, source); err != nil {
		t.Fatal(err)
	}
	base64PNG := base64.StdEncoding.EncodeToString(encodedPNG.Bytes())
	logo, err := decodeRestaurantLogo(base64PNG, "image/png")
	if err != nil {
		t.Fatalf("decodeRestaurantLogo() error = %v", err)
	}
	if logo.ContentType != "image/png" || !bytes.Equal(logo.Data, encodedPNG.Bytes()) {
		t.Fatal("decodeRestaurantLogo() did not preserve the validated PNG")
	}
	for _, invalid := range []struct {
		encoded     string
		contentType string
	}{
		{encoded: base64PNG, contentType: "image/jpeg"},
		{encoded: "not-base64", contentType: "image/png"},
		{encoded: "", contentType: "image/png"},
	} {
		if _, err := decodeRestaurantLogo(invalid.encoded, invalid.contentType); err == nil {
			t.Errorf("decodeRestaurantLogo(%q, %q) accepted invalid logo", invalid.encoded[:min(len(invalid.encoded), 16)], invalid.contentType)
		}
	}
}

func TestAddLogoToQRCodeKeepsDimensionsAndPlacesLogoAtCenter(t *testing.T) {
	qr, err := qrcode.Encode("https://example.test/r/sample", qrcode.High, 256)
	if err != nil {
		t.Fatal(err)
	}
	logo := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			logo.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var logoPNG bytes.Buffer
	if err := png.Encode(&logoPNG, logo); err != nil {
		t.Fatal(err)
	}

	withLogo, err := addLogoToQRCode(qr, logoPNG.Bytes())
	if err != nil {
		t.Fatalf("addLogoToQRCode() error = %v", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(withLogo))
	if err != nil {
		t.Fatalf("decode logo QR: %v", err)
	}
	original, _, err := image.Decode(bytes.NewReader(qr))
	if err != nil {
		t.Fatalf("decode original QR: %v", err)
	}
	if decoded.Bounds() != original.Bounds() {
		t.Fatalf("QR bounds = %v, want %v", decoded.Bounds(), original.Bounds())
	}
	center := decoded.At(decoded.Bounds().Dx()/2, decoded.Bounds().Dy()/2)
	if red, green, blue, _ := center.RGBA(); red <= green || red <= blue {
		t.Fatalf("center pixel = %v, want logo color", center)
	}
}

func publicRestaurantTestHandler(reader publicRestaurantReader) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	registerPublicRestaurantRoutes(mux, reader, logger)
	return mux
}
