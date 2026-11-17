// theme toggle: explicit choice wins over prefers-color-scheme, remembered per browser
function toggleTheme() {
  var root = document.documentElement;
  var dark = root.dataset.theme === 'dark' ||
    (!root.dataset.theme && matchMedia('(prefers-color-scheme: dark)').matches);
  root.dataset.theme = dark ? 'light' : 'dark';
  try { localStorage.setItem('theme', root.dataset.theme); } catch (e) {}
}

// syntax highlighting (no-op if the CDN failed to load)
if (window.hljs) hljs.highlightAll();

// copy button on every code block
document.querySelectorAll('pre').forEach(function (pre) {
  var btn = document.createElement('button');
  btn.className = 'copy';
  btn.textContent = 'Copy';
  btn.addEventListener('click', function () {
    navigator.clipboard.writeText(pre.querySelector('code').innerText).then(function () {
      btn.textContent = 'Copied';
      setTimeout(function () { btn.textContent = 'Copy'; }, 1500);
    });
  });
  pre.appendChild(btn);
});

// anchor links on headings that have an id
document.querySelectorAll('h2[id], h3[id]').forEach(function (h) {
  var a = document.createElement('a');
  a.className = 'anchor';
  a.href = '#' + h.id;
  a.textContent = '#';
  h.appendChild(a);
});
