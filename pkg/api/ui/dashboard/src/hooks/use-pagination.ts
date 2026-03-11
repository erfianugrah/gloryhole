import { useState, useMemo, useCallback } from "react";

export interface UsePaginationOptions {
  totalItems: number;
  initialPage?: number;
  initialPageSize?: number;
  pageSizeOptions?: number[];
}

export function usePagination({
  totalItems,
  initialPage = 1,
  initialPageSize = 25,
  pageSizeOptions = [10, 25, 50, 100],
}: UsePaginationOptions) {
  const [page, setPage] = useState(initialPage);
  const [pageSize, setPageSize] = useState(initialPageSize);

  const totalPages = useMemo(
    () => Math.max(1, Math.ceil(totalItems / pageSize)),
    [totalItems, pageSize]
  );

  const offset = useMemo(() => (page - 1) * pageSize, [page, pageSize]);

  const goToPage = useCallback(
    (p: number) => setPage(Math.max(1, Math.min(p, totalPages))),
    [totalPages]
  );

  const nextPage = useCallback(
    () => goToPage(page + 1),
    [page, goToPage]
  );

  const prevPage = useCallback(
    () => goToPage(page - 1),
    [page, goToPage]
  );

  const changePageSize = useCallback(
    (size: number) => {
      setPageSize(size);
      setPage(1);
    },
    []
  );

  return {
    page,
    pageSize,
    totalPages,
    offset,
    pageSizeOptions,
    goToPage,
    nextPage,
    prevPage,
    changePageSize,
    hasPrev: page > 1,
    hasNext: page < totalPages,
  };
}
