// Apply the saved appearance before styles and the application load.
;(function () {
  var mode = 'dark'
  try {
    var saved = localStorage.getItem('pod-studio.theme')
    if (saved === 'light' || saved === 'dark') mode = saved
  } catch (_) {}
  document.documentElement.dataset.themeMode = mode
  document.documentElement.dataset.theme = mode
  document.documentElement.style.colorScheme = mode
})()
