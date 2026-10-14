// Scope IDs stay strings so int64 targets are never rounded by JavaScript.
export function parseConversationID(scope) {
  const match = /^(private|group):([1-9][0-9]*)$/.exec(scope || '')
  if (!match || match[2].length > 19 || BigInt(match[2]) > 9223372036854775807n) return null
  return { type: match[1], id: match[2], key: match[1] === 'group' ? 'g' + match[2] : match[2] }
}
