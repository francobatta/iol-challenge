/**
 * Returns the user IDs in text someone typed or pasted: separated by commas, spaces or
 * new lines, each one once, in the order they first appear.
 */
export function parseIDs(text: string): string[] {
  return [...new Set(text.split(/[\s,]+/).filter((id) => id !== ""))]
}
