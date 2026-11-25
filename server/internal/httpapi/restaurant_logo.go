package httpapi

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"log/slog"
	"net/http"
)

const (
	maxRestaurantLogoBytes = 512 * 1024
	maxRestaurantLogoEdge  = 2048
)

type restaurantLogo struct {
	Data        []byte
	ContentType string
}

func decodeRestaurantLogo(encoded, contentType string) (*restaurantLogo, error) {
	if encoded == "" && contentType == "" {
		return nil, nil
	}
	if encoded == "" || (contentType != "image/png" && contentType != "image/jpeg") {
		return nil, invalidApplication("Choose a PNG or JPEG restaurant logo.")
	}
	maxEncodedBytes := base64.StdEncoding.EncodedLen(maxRestaurantLogoBytes)
	if len(encoded) > maxEncodedBytes {
		return nil, invalidApplication("The logo must be 512 KB or smaller.")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > maxRestaurantLogoBytes {
		return nil, invalidApplication("The logo must be a valid PNG or JPEG image no larger than 512 KB.")
	}
	detectedType := http.DetectContentType(data)
	if detectedType != contentType {
		return nil, invalidApplication("The logo file content does not match its image type.")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 32 || config.Height < 32 ||
		config.Width > maxRestaurantLogoEdge || config.Height > maxRestaurantLogoEdge ||
		int64(config.Width)*int64(config.Height) > 4_194_304 {
		return nil, invalidApplication("Use a readable logo image between 32 and 2048 pixels per side.")
	}
	return &restaurantLogo{Data: data, ContentType: contentType}, nil
}

func addLogoToQRCode(qrData, logoData []byte) ([]byte, error) {
	if len(logoData) == 0 {
		return qrData, nil
	}
	qrImage, _, err := image.Decode(bytes.NewReader(qrData))
	if err != nil {
		return nil, fmt.Errorf("decode generated restaurant QR: %w", err)
	}
	logoImage, _, err := image.Decode(bytes.NewReader(logoData))
	if err != nil {
		return nil, fmt.Errorf("decode restaurant logo for QR: %w", err)
	}

	canvas := image.NewRGBA(qrImage.Bounds())
	draw.Draw(canvas, canvas.Bounds(), qrImage, qrImage.Bounds().Min, draw.Src)
	qrSize := canvas.Bounds().Dx()
	logoSide := qrSize / 6
	padding := qrSize / 80
	plate := image.Rect(
		(canvas.Bounds().Dx()-logoSide-2*padding)/2,
		(canvas.Bounds().Dy()-logoSide-2*padding)/2,
		(canvas.Bounds().Dx()+logoSide+2*padding)/2,
		(canvas.Bounds().Dy()+logoSide+2*padding)/2,
	)
	draw.Draw(canvas, plate, image.NewUniform(color.White), image.Point{}, draw.Src)

	logoBounds := logoImage.Bounds()
	cropSide := logoBounds.Dx()
	if logoBounds.Dy() < cropSide {
		cropSide = logoBounds.Dy()
	}
	crop := image.Rect(
		logoBounds.Min.X+(logoBounds.Dx()-cropSide)/2,
		logoBounds.Min.Y+(logoBounds.Dy()-cropSide)/2,
		logoBounds.Min.X+(logoBounds.Dx()+cropSide)/2,
		logoBounds.Min.Y+(logoBounds.Dy()+cropSide)/2,
	)
	resized := image.NewRGBA(image.Rect(0, 0, logoSide, logoSide))
	for y := 0; y < logoSide; y++ {
		for x := 0; x < logoSide; x++ {
			source := image.Pt(
				crop.Min.X+x*crop.Dx()/logoSide,
				crop.Min.Y+y*crop.Dy()/logoSide,
			)
			resized.Set(x, y, logoImage.At(source.X, source.Y))
		}
	}
	logoRect := image.Rect(
		(canvas.Bounds().Dx()-logoSide)/2,
		(canvas.Bounds().Dy()-logoSide)/2,
		(canvas.Bounds().Dx()+logoSide)/2,
		(canvas.Bounds().Dy()+logoSide)/2,
	)
	draw.Draw(canvas, logoRect, resized, image.Point{}, draw.Over)

	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode restaurant QR with logo: %w", err)
	}
	return output.Bytes(), nil
}

func writeRestaurantLogo(
	w http.ResponseWriter,
	r *http.Request,
	logger *slog.Logger,
	data []byte,
	contentType string,
	cacheControl string,
) {
	if len(data) == 0 || (contentType != "image/png" && contentType != "image/jpeg") {
		writeMappedError(w, r, logger, fmt.Errorf("restaurant logo data is invalid"))
		return
	}
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		loggerFromContext(r.Context(), logger).ErrorContext(
			r.Context(), "write restaurant logo", "error", err,
		)
	}
}
