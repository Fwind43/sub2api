export interface UpstreamModelMapping { from: string; to: string }

// Strip only the provider segment, and retain the full ID as the route target.
export function mergeUpstreamModelAliases(
  upstream: string[], selected: string[], existing: UpstreamModelMapping[], refresh = false
): { models: string[]; mappings: UpstreamModelMapping[] } {
  const ids = [...new Set(upstream.map(id => id.trim()).filter(Boolean))]
  const separatorOf = (id: string) => id.search(/[:/]/)
  const aliasOf = (id: string) => {
    const separator = separatorOf(id)
    return separator > 0 && separator < id.length - 1 ? id.slice(separator + 1) : id
  }
  // Only prune aliases previously generated for this upstream provider.
  // Other selected entries and custom routes remain under the admin's control.
  const providers = new Set(ids.filter(id => separatorOf(id) > 0).map(id => id.slice(0, separatorOf(id))))
  const stale = refresh ? existing.filter(mapping => {
    const slash = separatorOf(mapping.to)
    return slash > 0 && providers.has(mapping.to.slice(0, slash))
      && mapping.from === aliasOf(mapping.to) && !ids.includes(mapping.to)
  }) : []
  const staleNames = new Set(stale.map(mapping => mapping.from))
  const mappings = existing.filter(mapping => !stale.includes(mapping)).map(mapping => ({ ...mapping }))
  const models = selected.filter(model => !staleNames.has(model))
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
