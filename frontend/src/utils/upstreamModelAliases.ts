export interface UpstreamModelMapping { from: string; to: string }

// Strip only the provider segment, and retain the full ID as the route target.
export function mergeUpstreamModelAliases(
  upstream: string[], selected: string[], existing: UpstreamModelMapping[]
): { models: string[]; mappings: UpstreamModelMapping[] } {
  const ids = [...new Set(upstream.map(id => id.trim()).filter(Boolean))]
  const mappings = existing.map(mapping => ({ ...mapping }))
  const models = [...selected]
  const aliasOf = (id: string) => {
    const slash = id.indexOf('/')
    return slash > 0 && slash < id.length - 1 ? id.slice(slash + 1) : id
  }
  const counts = new Map<string, number>()
  for (const id of ids) counts.set(aliasOf(id), (counts.get(aliasOf(id)) ?? 0) + 1)
  for (const id of ids) {
    const alias = aliasOf(id)
    const occupied = mappings.find(mapping => mapping.from === alias)
    const original = mappings.find(mapping => mapping.from === id)
    const safe = alias !== id && counts.get(alias) === 1 && !ids.includes(alias)
      && (!models.includes(alias) || occupied?.to === id)
      && (!occupied || occupied.to === id)
      && (!original || original.to === id)
      && !mappings.some(mapping => mapping.from.includes('*'))
    const name = safe ? alias : id
    if (safe) {
      const index = models.indexOf(id)
      if (index >= 0) models.splice(index, 1)
      const identity = mappings.findIndex(mapping => mapping.from === id && mapping.to === id)
      if (identity >= 0) mappings.splice(identity, 1)
      if (!occupied) mappings.push({ from: alias, to: id })
    }
    if (!models.includes(name)) models.push(name)
  }
  return { models, mappings }
}
