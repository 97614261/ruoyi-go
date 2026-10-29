// Preserve the user's page length, but discard hidden route filters as well as
// visible fields. Defaults describe the current list, never a pending write.
export function resetERPQuery(query, defaults = {}) {
  const pageSize = query.pageSize ?? defaults.pageSize ?? 10
  for (const key of Object.keys(query)) delete query[key]
  Object.assign(query, defaults, { pageNum: 1, pageSize })
}
