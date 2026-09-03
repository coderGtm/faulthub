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
	});
})();