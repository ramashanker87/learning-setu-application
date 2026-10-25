package transcription

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os/exec"
	"testing"
)

func TestRecordingDurationBoundaries(t *testing.T) {
	for _, seconds := range []float64{-1, 0, 19.99, 20, 30, 60, 60.01, 90, math.NaN(), math.Inf(1)} {
		valid := seconds >= 20 && seconds <= 60
		if ValidRecordingDuration(seconds) != valid {
			t.Errorf("duration %v", seconds)
		}
	}
}

func TestActualAudioDuration(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe required; run this test in the runtime image")
	}
	for _, seconds := range []int{19, 20, 60, 61} {
		data := make([]byte, 44+seconds*32000)
		copy(data, "RIFF")
		binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
		copy(data[8:], "WAVEfmt ")
		binary.LittleEndian.PutUint32(data[16:], 16)
		binary.LittleEndian.PutUint16(data[20:], 1)
		binary.LittleEndian.PutUint16(data[22:], 1)
		binary.LittleEndian.PutUint32(data[24:], 16000)
		binary.LittleEndian.PutUint32(data[28:], 32000)
		binary.LittleEndian.PutUint16(data[32:], 2)
		binary.LittleEndian.PutUint16(data[34:], 16)
		copy(data[36:], "data")
		binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
		duration, err := Validate(context.Background(), data, "audio/wav", 300)
		if seconds < 20 || seconds > 60 {
			if !errors.Is(err, ErrRecordingDuration) {
				t.Errorf("%ds: expected duration rejection, got %v", seconds, err)
			}
		} else if err != nil || duration != float64(seconds) {
			t.Errorf("%ds: got %v, %v", seconds, duration, err)
		}
	}
}
