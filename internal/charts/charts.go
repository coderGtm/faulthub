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
	base := float64(height - 30)
	var b strings.Builder
	b.WriteString(svgOpen(width, height))
	axes(&b, width, height, plotH, top)
	for i := 0; i <= 4; i++ {
		y := base - float64(plotH*i)/4
		b.WriteString(`<line class="chart-grid" x1="34" y1="` + f1(y) +
			`" x2="` + strconv.Itoa(34+plotW) + `" y2="` + f1(y) + `"/>`)
	}
	step := float64(plotW) / float64(len(points)-1)
	labelEvery := len(points)/8 + 1
	var coords []string
	for i, p := range points {
		x := 34.0 + float64(i)*step
		y := base - (float64(p.Value)/float64(top))*float64(plotH)
		coords = append(coords, coord(x, y))
		if i%labelEvery == 0 || i == len(points)-1 {
			b.WriteString(text(x, float64(height-12), p.Label, "middle"))
		}
	}
	b.WriteString(`<polygon class="chart-area" points="` + strings.Join(coords, " ") +
		" " + coord(34, base) + " " + coord(34+float64(plotW), base) + `"/>`)
	b.WriteString(`<polyline class="chart-line" points="` + strings.Join(coords, " ") + `"/>`)
	for i, p := range points {
		if p.Value == 0 {
			continue
		}
		x := 34.0 + float64(i)*step
		y := base - (float64(p.Value)/float64(top))*float64(plotH)
		b.WriteString(`<circle class="chart-dot" cx="` + f1(x) + `" cy="` + f1(y) + `" r="2.6"><title>` +
			html.EscapeString(p.Label+": "+strconv.FormatInt(p.Value, 10)) + `</title></circle>`)
	}
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
			`" width="` + strconv.Itoa(w) + `" height="` + strconv.Itoa(rowH-8) + `" rx="3"><title>` +
			html.EscapeString(bar.Label+": "+strconv.FormatInt(bar.Value, 10)) + `</title></rect>`)
		b.WriteString(text(8, float64(yMid+4), truncate(bar.Label, 24), "start"))
		b.WriteString(text(float64(170+w+8), float64(yMid+4), strconv.FormatInt(bar.Value, 10), "start"))
	}
	b.WriteString("</svg>")
	return template.HTML(b.String())
}

func Donut(bars []Bar, width, height int) template.HTML {
	var total int64
	for _, bar := range bars {
		total += bar.Value
	}
	if len(bars) == 0 || total == 0 {
		return emptyState(width, height)
	}
	shown := bars
	var other int64
	if len(shown) > 6 {
		shown = bars[:6]
		for _, bar := range bars[6:] {
			other += bar.Value
		}
	}
	cx, cy, r := 90.0, float64(height)/2, math.Min(64, float64(height)/2-12)
	lx := 196.0
	var b strings.Builder
	b.WriteString(svgOpen(width, height))
	var offset float64
	slot := func(i int) string { return "chart-slice-" + strconv.Itoa(i%6+1) }
	seg := func(label string, value int64, i int) {
		pct := float64(value) / float64(total) * 100
		b.WriteString(`<circle class="` + slot(i) + `" cx="` + f1(cx) + `" cy="` + f1(cy) +
			`" r="` + f1(r) + `" fill="transparent" stroke-width="26" pathLength="100" stroke-dasharray="` +
			f1(pct) + ` 100" stroke-dashoffset="` + f1(-offset) +
			`" transform="rotate(-90 ` + f1(cx) + ` ` + f1(cy) + `)"><title>` +
			html.EscapeString(label+": "+strconv.FormatInt(value, 10)) + `</title></circle>`)
		offset += pct
	}
	for i, bar := range shown {
		seg(bar.Label, bar.Value, i)
	}
	if other > 0 {
		seg("Other", other, len(shown))
	}
	b.WriteString(text(cx, cy+6, strconv.FormatInt(total, 10), "middle"))
	ly := cy - float64(len(shown)+b2i(other > 0)-1)*13
	for i, bar := range shown {
		pct := float64(bar.Value) / float64(total) * 100
		legendRow(&b, lx, ly+float64(i*26), i, truncate(bar.Label, 22), pct)
	}
	if other > 0 {
		legendRow(&b, lx, ly+float64(len(shown)*26), len(shown), "Other", float64(other)/float64(total)*100)
	}
	b.WriteString("</svg>")
	return template.HTML(b.String())
}

func legendRow(b *strings.Builder, x, y float64, i int, label string, pct float64) {
	cls := "chart-swatch-" + strconv.Itoa(i%6+1)
	b.WriteString(`<rect class="` + cls + `" x="` + f1(x) + `" y="` + f1(y-9) +
		`" width="11" height="11" rx="3"/>`)
	b.WriteString(text(x+17, y+1, label, "start"))
	b.WriteString(text(x+172, y+1, strconv.FormatFloat(pct, 'f', 1, 64)+"%", "end"))
}

func b2i(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

func f1(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64)
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
