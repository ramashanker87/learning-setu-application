// Package scoring preserves the POC's Unicode word alignment and ASR-derived metrics.
package scoring

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"math"
	"strings"
	"unicode"
)

func Words(text string) []string {
	return strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) {
			return r
		}
		return ' '
	}, cases.Fold().String(norm.NFC.String(text))))
}
func Score(expected, transcript string, mock bool) map[string]any {
	e, w := Words(expected), Words(transcript)
	cols := len(w) + 1
	cost := make([]int, (len(e)+1)*cols)
	op := make([]string, len(cost))
	for i := 1; i <= len(e); i++ {
		cost[i*cols] = i
		op[i*cols] = "skipped"
	}
	for j := 1; j <= len(w); j++ {
		cost[j] = j
		op[j] = "inserted"
	}
	for i := 1; i <= len(e); i++ {
		for j := 1; j <= len(w); j++ {
			idx := i*cols + j
			cost[idx] = cost[idx-cols-1] + 1
			op[idx] = "substituted"
			if e[i-1] == w[j-1] {
				cost[idx]--
				op[idx] = "match"
			}
			if x := cost[idx-cols] + 1; x < cost[idx] {
				cost[idx] = x
				op[idx] = "skipped"
			}
			if x := cost[idx-1] + 1; x < cost[idx] {
				cost[idx] = x
				op[idx] = "inserted"
			}
		}
	}
	errors := []map[string]any{}
	matches := 0
	i, j := len(e), len(w)
	for i > 0 || j > 0 {
		kind := op[i*cols+j]
		row := map[string]any{"error_type": kind, "expected_word": nil, "spoken_word": nil}
		switch kind {
		case "match", "substituted":
			i--
			j--
			row["expected_word"] = e[i]
			row["spoken_word"] = w[j]
		case "skipped":
			i--
			row["expected_word"] = e[i]
		case "inserted":
			j--
			row["spoken_word"] = w[j]
		}
		row["position"] = i
		if kind == "match" {
			matches++
		} else {
			errors = append(errors, row)
		}
	}
	for i, j := 0, len(errors)-1; i < j; i, j = i+1, j-1 {
		errors[i], errors[j] = errors[j], errors[i]
	}
	accuracy, pronunciation := 0.0, 0.0
	if len(e) > 0 {
		accuracy = math.Max(0, (1-float64(len(errors))/float64(len(e)))*100)
		pronunciation = float64(matches) / float64(len(e)) * 100
		if mock {
			pronunciation *= .95
		}
	}
	var pauses any = nil
	if mock {
		pauses = 0
	}
	return map[string]any{"transcript": strings.Join(w, " "), "accuracy_score": math.RoundToEven(accuracy*100) / 100, "pronunciation_score": math.RoundToEven(pronunciation*100) / 100, "pause_count": pauses, "errors": errors}
}
