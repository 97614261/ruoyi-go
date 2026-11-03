// The dictionary supplies labels, never new server behavior or executable component kinds.
export function statusChoices(rows = []) { return ['0','1'].map(value => ({value,label:rows.find(r=>r.value===value)?.label || (value==='0'?'启用':'停用')})) }
