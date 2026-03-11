import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { T } from "@/lib/typography";

interface TablePaginationProps {
  page: number;
  totalPages: number;
  pageSize: number;
  pageSizeOptions: number[];
  hasPrev: boolean;
  hasNext: boolean;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  totalItems?: number;
}

export function TablePagination({
  page,
  totalPages,
  pageSize,
  pageSizeOptions,
  hasPrev,
  hasNext,
  onPageChange,
  onPageSizeChange,
  totalItems,
}: TablePaginationProps) {
  return (
    <div className="flex items-center justify-between px-2 py-3">
      <div className="flex items-center gap-2">
        <span className={T.muted}>Rows per page</span>
        <Select
          value={String(pageSize)}
          onValueChange={(v) => onPageSizeChange(Number(v))}
        >
          <SelectTrigger className="h-8 w-[70px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {pageSizeOptions.map((size) => (
              <SelectItem key={size} value={String(size)}>
                {size}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-center gap-2">
        {totalItems !== undefined && (
          <span className={T.muted}>
            {totalItems.toLocaleString()} total
          </span>
        )}
        <span className={T.muted}>
          Page {page} of {totalPages}
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          disabled={!hasPrev}
          onClick={() => onPageChange(page - 1)}
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          disabled={!hasNext}
          onClick={() => onPageChange(page + 1)}
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
