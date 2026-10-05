/**
 * The sort keys the backend accepts.
 *
 * `internal/store/query.go` sorts the fixed A..O index block by `c1..c15` —
 * one-based, matching the column letters — and a week column by `wk_<code>`.
 * Any other key is silently swapped for `seq`, which leaves the sort arrow
 * showing over rows that are not sorted by that column at all, so these are the
 * only two forms the grid may send.
 */

/** The sort key for an index column, from its zero-based position in A..O. */
export const indexSortKey = (index: number): string => `c${index + 1}`;

/** The sort key for a week column. */
export const weekSortKey = (code: string): string => `wk_${code}`;
