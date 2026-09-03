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