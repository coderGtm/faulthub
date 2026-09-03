// Package charts renders SVG charts server-side.
package charts

import (
	"html"
	"html/template"
	"math"
	"strconv"
	"strings"
)

type Point struct {
	Label string
	Value int64
}

type Bar struct {
	Label string
	Value int64
}

func Line(points []Point, width, height int) template.HTML {
	var maxV int64
	anyPositive := false
	for _, p := range points {
		if p.Value > maxV {
			maxV = p.Value
		}
		if p.Value > 0 {
			anyPositive = true
		}
	}
	if len(points) < 2 || !anyPositive {
		return emptyState(width, height)
	}
	top := niceMax(maxV)
	plotW := width - 34 - 46
	plotH := height - 30 - 8
	var b strings.Builder
	b.WriteString(svgOpen(width, height))
	axes(&b, width, height, plotH, top)
	step := float64(plotW) / float64(len(points)-1)
	labelEvery := len(points)/8 + 1
	var coords []string
	for i, p := range points {
		x := 34.0 + float64(i)*step
		y := float64(height-30) - (float64(p.Value)/float64(top))*float64(plotH)
		coords = append(coords, coord(x, y))
		if i%labelEvery == 0 || i == len(points)-1 {
			b.WriteString(text(x, float64(height-12), p.Label, "middle"))
		}
	}
	b.WriteString(`<polygon class="chart-area" points="` + strings.Join(coords, " ") +
		" " + coord(34, float64(height-30)) + " " + coord(34+float64(plotW), float64(height-30)) + `"/>`)
	b.WriteString(`<polyline class="chart-line" points="` + strings.Join(coords, " ") + `"/>`)
	b.WriteString("</svg>")
	return template.HTML(b.String())
}

func Bars(bars []Bar, width, height int) template.HTML {
	if len(bars) == 0 {
		return emptyState(width, height)
	}
	var maxV int64
	for _, bar := range bars {
		if bar.Value > maxV {
			maxV = bar.Value
		}
	}
	if maxV == 0 {
		maxV = 1
	}
	rowH := height / len(bars)
	barMax := width - 180
	var b strings.Builder
	b.WriteString(svgOpen(width, height))
	for i, bar := range bars {
		yMid := i*rowH + rowH/2
		w := int(math.Round(float64(bar.Value) / float64(maxV) * float64(barMax)))
		b.WriteString(`<rect class="chart-bar" x="170" y="` + strconv.Itoa(i*rowH+4) +
			`" width="` + strconv.Itoa(w) + `" height="` + strconv.Itoa(rowH-8) + `"/>`)
		b.WriteString(text(8, float64(yMid+4), truncate(bar.Label, 24), "start"))
		b.WriteString(text(float64(170+w+8), float64(yMid+4), strconv.FormatInt(bar.Value, 10), "start"))
	}
	b.WriteString("</svg>")
	return template.HTML(b.String())
}

func emptyState(width, height int) template.HTML {
	return template.HTML(svgOpen(width, height) +
		text(float64(width)/2, float64(height)/2, "no data", "middle") + "</svg>")
}

func svgOpen(width, height int) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` +
		strconv.Itoa(width) + " " + strconv.Itoa(height) + `" class="chart" role="img">`
}

func axes(b *strings.Builder, width, height, plotH int, top int64) {
	b.WriteString(`<line class="chart-axis" x1="34" y1="` + strconv.Itoa(height-30) +
		`" x2="` + strconv.Itoa(width-40) + `" y2="` + strconv.Itoa(height-30) + `"/>`)
	b.WriteString(`<line class="chart-axis" x1="34" y1="6" x2="34" y2="` + strconv.Itoa(height-30) + `"/>`)
	for i := 0; i <= 4; i++ {
		v := top * int64(i) / 4
		y := (height - 30) - (plotH*i)/4
		b.WriteString(text(28, float64(y+4), strconv.FormatInt(v, 10), "end"))
	}
}

func text(x, y float64, s, anchor string) string {
	return `<text x="` + strconv.FormatFloat(x, 'f', 1, 64) +
		`" y="` + strconv.FormatFloat(y, 'f', 1, 64) +
		`" class="chart-text" text-anchor="` + anchor + `">` +
		html.EscapeString(s) + `</text>`
}

func coord(x, y float64) string {
	return strconv.FormatFloat(x, 'f', 1, 64) + "," + strconv.FormatFloat(y, 'f', 1, 64)
}

func niceMax(v int64) int64 {
	if v <= 5 {
		return 5
	}
	pow := int64(1)
	for v > 5 {
		v /= 5
		pow *= 5
	}
	return v * pow * 5
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
