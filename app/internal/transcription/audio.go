package transcription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const MinRecordingSeconds = 20
const MaxRecordingSeconds = 60

var ErrRecordingDuration = errors.New("Recording must be between 20 and 60 seconds. Record again.")

func ValidRecordingDuration(seconds float64) bool {
	return !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds >= MinRecordingSeconds && seconds <= MaxRecordingSeconds
}

var ErrTemporaryStorage = errors.New("temporary audio storage unavailable")

// Validate decodes untrusted media with a deadline; claimed browser duration is never authoritative.
func Validate(ctx context.Context, data []byte, mediaType string, maxSeconds int) (float64, error) {
	f, e := os.CreateTemp("", "setu-audio-*")
	if e != nil {
		return 0, fmt.Errorf("%w: %w", ErrTemporaryStorage, e)
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return 0, fmt.Errorf("%w: %w", ErrTemporaryStorage, e)
	}
	f.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "ffprobe", append([]string{"-protocol_whitelist", "file,pipe"}, args...)...)
		return cmd.Output()
	}
	out, e := run("-v", "error", "-show_format", "-show_streams", "-of", "json", f.Name())
	if e != nil {
		return 0, fmt.Errorf("audio decoding failed")
	}
	var meta struct {
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			Type string `json:"codec_type"`
		} `json:"streams"`
	}
	if e = json.Unmarshal(out, &meta); e != nil {
		return 0, e
	}
	audio := false
	for _, st := range meta.Streams {
		if st.Type == "video" {
			return 0, fmt.Errorf("video is not allowed")
		}
		audio = audio || st.Type == "audio"
	}
	format := map[string]string{"audio/wav": "wav", "audio/webm": "webm", "audio/mp4": "mp4", "audio/ogg": "ogg"}[mediaType]
	duration, _ := strconv.ParseFloat(meta.Format.Duration, 64)
	if duration == 0 {
		out, e = run("-v", "error", "-select_streams", "a:0", "-show_entries", "packet=pts_time,duration_time", "-of", "json", f.Name())
		if e != nil {
			return 0, e
		}
		var frames struct {
			Packets []struct {
				PTS      string `json:"pts_time"`
				Duration string `json:"duration_time"`
			} `json:"packets"`
		}
		if e = json.Unmarshal(out, &frames); e != nil {
			return 0, e
		}
		for _, p := range frames.Packets {
			x, _ := strconv.ParseFloat(p.PTS, 64)
			d, _ := strconv.ParseFloat(p.Duration, 64)
			if x+d > duration {
				duration = x + d
			}
		}
	}
	if !ValidRecordingDuration(duration) {
		return 0, ErrRecordingDuration
	}
	if !audio || format == "" || !strings.Contains(meta.Format.Name, format) || duration <= 0 || duration > float64(maxSeconds) {
		return 0, fmt.Errorf("audio duration or format invalid")
	}
	return duration, nil
}
