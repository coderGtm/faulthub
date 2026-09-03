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

func TestBarsSingleValueFullyVisible(t *testing.T) {
	svg := string(Bars([]Bar{{Label: "1.0", Value: 12}}, 500, 288))
	if !strings.Contains(svg, `viewBox="0 0 500 48"`) {
		t.Fatal("single bar must collapse to one compact row")
	}
	if !strings.Contains(svg, `text-anchor="end"`) {
		t.Fatal("value must be end-anchored inside the viewport")
	}
	if !strings.Contains(svg, `>12</text>`) {
		t.Fatal("value must render")
	}
}

func TestDonutRenders(t *testing.T) {
	bars := []Bar{{Label: "14", Value: 12}, {Label: "15", Value: 7}, {Label: "13", Value: 1}}
	svg := string(Donut(bars, 500, 180))
	for _, want := range []string{"<svg", "</svg>", "14", "60.0%", "20", "chart-slice-1", "<title>"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestDonutEscapesLabels(t *testing.T) {
	bars := []Bar{{Label: `"><script>`, Value: 1}}
	if svg := string(Donut(bars, 500, 180)); strings.Contains(svg, "<script>") {
		t.Fatal("label must be escaped")
	}
}

func TestDonutEmpty(t *testing.T) {
	if svg := string(Donut(nil, 500, 180)); !strings.Contains(svg, "no data") {
		t.Fatalf("empty state: %s", svg)
	}
	if svg := string(Donut([]Bar{{Label: "a", Value: 0}}, 500, 180)); !strings.Contains(svg, "no data") {
		t.Fatalf("zero total must render empty state: %s", svg)
	}
}

func TestDonutGroupsOverflow(t *testing.T) {
	bars := []Bar{
		{Label: "v1", Value: 8}, {Label: "v2", Value: 7}, {Label: "v3", Value: 6},
		{Label: "v4", Value: 5}, {Label: "v5", Value: 4}, {Label: "v6", Value: 3},
		{Label: "v7", Value: 2}, {Label: "v8", Value: 1},
	}
	svg := string(Donut(bars, 500, 200))
	if !strings.Contains(svg, "Other") {
		t.Fatal("overflow slices must group into Other")
	}
	if strings.Contains(svg, "v8") {
		t.Fatal("grouped labels must not render individually")
	}
}
