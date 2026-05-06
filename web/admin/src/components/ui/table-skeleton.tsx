import { cn } from "@/lib/utils";
import { TableBody, TableCell, TableRow } from "@/components/ui/table";

interface TableSkeletonColumn {
  width: string;
  pill?: boolean;
  muted?: boolean;
  align?: "left" | "right" | "center";
}

interface TableSkeletonProps {
  columns: TableSkeletonColumn[];
  rows?: number;
}

export function TableSkeleton({ columns, rows = 4 }: TableSkeletonProps) {
  return (
    <TableBody>
      {Array.from({ length: rows }).map((_, rowIdx) => (
        <TableRow key={rowIdx}>
          {columns.map((col, colIdx) => (
            <TableCell
              key={colIdx}
              className={cn(
                col.align === "right" && "text-right",
                col.align === "center" && "text-center",
              )}
            >
              <div
                className={cn(
                  "animate-pulse",
                  col.width,
                  col.pill ? "h-5 rounded-full" : "h-4 rounded",
                  col.muted ? "bg-muted/60" : "bg-muted",
                  col.align === "right" && "ml-auto",
                  col.align === "center" && "mx-auto",
                )}
              />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </TableBody>
  );
}
