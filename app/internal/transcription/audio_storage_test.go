package transcription

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestAudioValidationReportsUnavailableScratchStorage(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-directory"))
	_, err := Validate(context.Background(), []byte("synthetic"), "audio/wav", 30)
	if !errors.Is(err, ErrTemporaryStorage) {
		t.Fatalf("expected storage diagnostic, got %v", err)
	}
}
