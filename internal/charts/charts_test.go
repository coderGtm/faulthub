package charts

import (
	"strings"
	"testing"
)

func TestLineRenders(t *testing.T) {
	pts := []Point{{Label: "2026-09-01", Value: 0}, {Label: "2026-09-02", Value: 5}, {Label: "2026-09-03", Value: 2}}
	svg := string(Line(pts, 600, 200))
	for _, want := range []string{"<svg", "</svg>", "2026-09-02", "<polyline", "chart-area"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestLineEscapesLabels(t *testing.T) {
	pts := []Point{{Label: `<script>x</script>`, Value: 1}, {Label: "b", Value: 2}}
	svg := string(Line(pts, 600, 200))
	if strings.Contains(svg, "<script>") {
		t.Fatal("label must be escaped")
	}
	if !strings.Contains(svg, "&lt;script&gt;") {
		t.Fatal("escaped label expected")
	}
}

func TestLineEmpty(t *testing.T) {
	if svg := string(Line(nil, 600, 200)); !strings.Contains(svg, "no data") {
		t.Fatalf("empty state: %s", svg)
	}
}

func TestLineAllZero(t *testing.T) {
	pts := []Point{{Label: "a", Value: 0}, {Label: "b", Value: 0}}
	if svg := string(Line(pts, 600, 200)); strings.Contains(svg, "<polyline") {
		t.Fatal("all-zero series must render empty state")
	}
}

func TestBarsRenders(t *testing.T) {
	bars := []Bar{{Label: "Pixel 8", Value: 12}, {Label: "Galaxy S23", Value: 7}}
	svg := string(Bars(bars, 500, 100))
	for _, want := range []string{"<svg", "Pixel 8", "Galaxy S23", "12", "7", "<rect"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestBarsEscapesLabels(t *testing.T) {
	bars := []Bar{{Label: `"><script>`, Value: 1}}
	if svg := string(Bars(bars, 500, 100)); strings.Contains(svg, "<script>") {
		t.Fatal("label must be escaped")
	}
}

func TestBarsEmpty(t *testing.T) {
	if svg := string(Bars(nil, 500, 100)); !strings.Contains(svg, "no data") {
		t.Fatalf("empty state: %s", svg)
	}
}
