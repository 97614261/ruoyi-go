export function localDate(date) {
  // Match the server's business timezone even on a browser in another timezone.
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(date)
  const value = key => parts.find(part => part.type === key).value
  return `${value('year')}-${value('month')}-${value('day')}`
}
export function formatMicros(raw) {
  const amount = BigInt(raw)
  return `${amount / 1000000n}.${(amount % 1000000n).toString().padStart(6, '0')}`
}
export function formatBytes(raw) {
  const bytes = BigInt(raw)
  if (bytes < 1024n) return `${bytes} B`
  for (const [unit, size] of [['GiB', 1073741824n], ['MiB', 1048576n], ['KiB', 1024n]]) {
    if (bytes >= size) return `${bytes / size}.${(bytes % size) * 10n / size} ${unit}`
  }
}
