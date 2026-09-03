document.addEventListener("click", function (e) {
	var b = e.target.closest("[data-copy]");
	if (!b) return;
	navigator.clipboard.writeText(b.dataset.copy).then(function () {
		var old = b.textContent;
		b.textContent = "copied";
		setTimeout(function () { b.textContent = old; }, 1200);
	});
});
document.addEventListener("submit", function (e) {
	var f = e.target.closest("form[data-confirm]");
	if (f && !window.confirm(f.dataset.confirm)) e.preventDefault();
});
document.querySelectorAll("time[datetime]").forEach(function (el) {
	var d = new Date(el.getAttribute("datetime"));
	if (isNaN(d)) return;
	el.textContent = d.toLocaleString(undefined, {
		month: "short", day: "numeric", year: "numeric",
		hour: "2-digit", minute: "2-digit"
	});
});
function fhThemeColors() {
	var cs = getComputedStyle(document.documentElement);
	var get = function (n) { return cs.getPropertyValue(n).trim(); };
	return {
		muted: get("--muted"), border: get("--border"),
		accent: get("--accent"), soft: get("--accent-soft"),
		line: get("--chart-line") || get("--accent"),
		fill: get("--chart-fill") || get("--accent-soft"),
		palette: [get("--c1"), get("--c2"), get("--c3"), get("--c4"), get("--c5"), get("--c6")]
	};
}
function fhBuildCharts() {
	if (typeof window.Chart === "undefined") return;
	var C = fhThemeColors();
	document.querySelectorAll("canvas.chartjs").forEach(function (cv) {
		var old = window.Chart.getChart(cv);
		if (old) old.destroy();
		var labels, values;
		try {
			labels = JSON.parse(cv.dataset.labels || "[]");
			values = JSON.parse(cv.dataset.values || "[]");
		} catch (err) { return; }
		if (!labels.length) return;
		var kind = cv.dataset.kind;
		var cfg;
		if (kind === "doughnut") {
			cfg = {
				type: "doughnut",
				data: { labels: labels, datasets: [{ data: values, backgroundColor: C.palette, borderWidth: 0 }] },
				options: {
					responsive: true, maintainAspectRatio: false, cutout: "62%",
					plugins: { legend: { position: "right", labels: { color: C.muted, boxWidth: 12, boxHeight: 12, padding: 14 } } }
				}
			};
		} else if (kind === "bar") {
			cfg = {
				type: "bar",
				data: { labels: labels, datasets: [{ data: values, backgroundColor: C.accent, borderRadius: 4, borderSkipped: false }] },
				options: {
					responsive: true, maintainAspectRatio: false, indexAxis: "y",
					plugins: { legend: { display: false } },
					scales: {
						x: { beginAtZero: true, ticks: { color: C.muted, precision: 0 }, grid: { color: C.border } },
						y: { ticks: { color: C.muted }, grid: { display: false } }
					}
				}
			};
		} else {
			cfg = {
				type: "line",
				data: { labels: labels, datasets: [{ data: values, borderColor: C.line, backgroundColor: C.fill, fill: true, tension: 0.35, pointRadius: 3, pointBackgroundColor: C.line, borderWidth: 2 }] },
				options: {
					responsive: true, maintainAspectRatio: false,
					plugins: { legend: { display: false } },
					scales: {
						x: { ticks: { color: C.muted, maxTicksLimit: 8 }, grid: { display: false } },
						y: { beginAtZero: true, ticks: { color: C.muted, precision: 0 }, grid: { color: C.border } }
					}
				}
			};
		}
		new window.Chart(cv, cfg);
	});
}
if (document.readyState === "loading") {
	document.addEventListener("DOMContentLoaded", fhBuildCharts);
} else {
	fhBuildCharts();
}
(function () {
	var root = document.documentElement;
	try {
		var saved = window.localStorage.getItem("fh-theme");
		if (saved === "light" || saved === "dark") root.setAttribute("data-theme", saved);
	} catch (err) {}
	var btn = document.getElementById("theme-toggle");
	if (!btn) return;
	btn.addEventListener("click", function () {
		var next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
		if (!root.getAttribute("data-theme")) {
			next = window.matchMedia("(prefers-color-scheme: dark)").matches ? "light" : "dark";
		}
		root.setAttribute("data-theme", next);
		try {
			window.localStorage.setItem("fh-theme", next);
		} catch (err) {}
		fhBuildCharts();
	});
})();