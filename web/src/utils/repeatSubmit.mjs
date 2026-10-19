// Keep the last request only in this page's memory. Request bodies may contain
// passwords: never persist them in sessionStorage/localStorage for this guard.
export function createRepeatSubmitGuard() {
  let previous
  return (request, interval = 1000) => {
    const elapsed = previous ? request.time - previous.time : -1
    if (previous && previous.url === request.url && previous.data === request.data && elapsed >= 0 && elapsed < interval) {
      return true
    }
    previous = { ...request }
    return false
  }
}
