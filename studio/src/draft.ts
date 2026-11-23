import { APIError } from './api'
import { clone, stable } from './graph'
import type { DraftDocument } from './contracts'

export type SaveState = 'saved' | 'dirty' | 'saving' | 'error' | 'conflict'
export class DraftSession {
  document: DraftDocument
  revision: string
  state: SaveState = 'saved'
  error = ''
  private baseline: string
  private timer: ReturnType<typeof setTimeout> | undefined
  private inflight: Promise<void> | null = null
  private disposed = false

  constructor(initial: DraftDocument, revision: string,
    private readonly save: (document: DraftDocument, revision: string) => Promise<{ revision: string }>,
    private readonly changed: () => void, private readonly delay = 800) {
    this.document = clone(initial); this.revision = revision; this.baseline = stable(initial)
  }
  get dirty() { return stable(this.document) !== this.baseline }
  edit(value: DraftDocument) {
    if (stable(value) === stable(this.document)) return
    this.document = clone(value)
    if (this.state !== 'conflict' && !this.inflight) { this.state = this.dirty ? 'dirty' : 'saved'; this.error = '' }
    this.changed(); this.schedule()
  }
  recover(value: DraftDocument, stale: boolean) {
    this.document = clone(value)
    this.state = stale ? 'conflict' : 'dirty'
    this.error = stale ? '本地草稿基于旧版本，请下载保留或另存为项目。' : ''
    this.changed(); this.schedule()
  }
  private schedule() {
    clearTimeout(this.timer)
    if (!this.disposed && this.dirty && this.state !== 'conflict' && this.state !== 'error' && !this.inflight) this.timer = setTimeout(() => { void this.flush() }, this.delay)
  }
  async flush(): Promise<void> {
    clearTimeout(this.timer)
    if (this.inflight) return this.inflight
    if (this.disposed || !this.dirty || this.state === 'conflict') return
    const submitted = clone(this.document), baseRevision = this.revision
    this.state = 'saving'; this.error = ''; this.changed()
    this.inflight = (async () => {
      try {
        const saved = await this.save(submitted, baseRevision)
        this.revision = saved.revision; this.baseline = stable(submitted)
        this.state = this.dirty ? 'dirty' : 'saved'
      } catch (error) {
        this.state = error instanceof APIError && error.code === 409 ? 'conflict' : 'error'
        this.error = error instanceof Error ? error.message : '保存失败，本地修改仍保留'
      } finally { this.inflight = null; this.changed(); this.schedule() }
    })()
    return this.inflight
  }
  dispose() { this.disposed = true; clearTimeout(this.timer) }
}
