export const sections = ['home', 'projects', 'templates', 'assets', 'runs', 'eligibility', 'product'] as const
export type Section = typeof sections[number]
export const productLocation = (hash: string) => hash.match(/^#product\/([1-9]\d{0,19})$/)?.[1] || ''
export function studioLocation(hash: string): { section: Section; projectId: string; runId: string } {
  if (productLocation(hash)) return { section: 'product', projectId: '', runId: '' }
  const match = hash.match(/^#(project|runs)\/([1-9]\d{0,19})$/)
  if (match) return { section: match[1] === 'runs' ? 'runs' : 'projects', projectId: match[1] === 'project' ? match[2]! : '', runId: match[1] === 'runs' ? match[2]! : '' }
  const name = hash.slice(1)
  if (name === 'nodes') return { section: 'templates', projectId: '', runId: '' }
  return { section: sections.includes(name as Section) ? name as Section : name ? 'projects' : 'home', projectId: '', runId: '' }
}
