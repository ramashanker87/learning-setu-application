package scoring

import "testing"

func TestAlignment(t *testing.T) {
	for _, c := range []struct {
		expected, spoken string
		score            float64
	}{{"One two three.", "one two three", 100}, {"one two three", "one three", 66.67}, {"one two", "one extra two", 50}, {"हम किताब पढ़ते हैं।", "हम किताब पढ़ते हैं", 100}, {"Straße", "STRASSE", 100}, {"", "", 0}} {
		r := Score(c.expected, c.spoken, false)
		if r["accuracy_score"] != c.score {
			t.Errorf("%q / %q: %v", c.expected, c.spoken, r)
		}
		if r["pause_count"] != nil {
			t.Fatal("missing timing must not imply zero pauses")
		}
	}
}
