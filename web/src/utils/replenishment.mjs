import { purchasePayload, immutablePayload } from './purchasing.mjs'
export function replenishmentPayload(form) {
  const body = purchasePayload(form)
  body.basis = form.lines.map(line => {
    if (!/^[0-9a-f]{64}$/.test(line.basis || '')) throw Error('补货建议依据已丢失，请重新查询选择')
    return { skuId: line.skuId, basis: line.basis }
  })
  return immutablePayload(body)
}
