import { useInfiniteQuery, type QueryKey } from "@tanstack/react-query"

import type { Page } from "@/lib/api"

/**
 * Loads a collection of the API a page at a time. `items` is everything loaded so far;
 * call fetchNextPage while hasNextPage to load more.
 */
export function usePages<T>(queryKey: QueryKey, fetchPage: (after?: string) => Promise<Page<T>>) {
  const result = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) => fetchPage(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_after,
  })
  return { ...result, items: result.data?.pages.flatMap((page) => page.items) ?? [] }
}

/**
 * Loads a whole collection, following pages up to maxPages of them. `complete` is false
 * if it stopped there with more to load.
 */
export async function loadAll<T>(
  fetchPage: (after?: string) => Promise<Page<T>>,
  maxPages: number,
): Promise<{ items: T[]; complete: boolean }> {
  const items: T[] = []
  let after: string | undefined
  for (let page = 0; page < maxPages; page++) {
    const { items: more, next_after } = await fetchPage(after)
    items.push(...more)
    if (!next_after) {
      return { items, complete: true }
    }
    after = next_after
  }
  return { items, complete: false }
}
