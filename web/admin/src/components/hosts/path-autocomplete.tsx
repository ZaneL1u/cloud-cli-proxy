import { useState, useRef, useEffect, useCallback } from "react";
import { Loader2, FolderOpen } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useHostFiles } from "@/hooks/use-host-files";
import { DirectoryChooserDialog } from "@/components/hosts/directory-chooser-dialog";
import { isAbsolutePath, isWindowsPath } from "@/lib/path-utils";

interface PathAutocompleteProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  showBrowseButton?: boolean;
  hostId?: string;
}

function getQueryAndFilter(value: string): { queryPath: string; filter: string } {
  if (isWindowsPath(value)) {
    const sep = value.includes("\\") ? "\\" : "/";
    const drive = value.substring(0, 2) + sep;
    const rest = value.slice(drive.length).replace(/[\\/]/g, sep).replace(new RegExp(`^[${sep}]+`), "");
    if (!rest) return { queryPath: drive, filter: "" };
    const lastSep = rest.lastIndexOf(sep);
    if (lastSep === -1) return { queryPath: drive, filter: rest };
    return {
      queryPath: drive + rest.slice(0, lastSep + 1),
      filter: rest.slice(lastSep + 1),
    };
  }

  if (value.endsWith("/")) {
    return { queryPath: value.slice(0, -1) || "/", filter: "" };
  }
  const lastSlash = value.lastIndexOf("/");
  if (lastSlash === -1) return { queryPath: "/", filter: value };
  return {
    queryPath: value.slice(0, lastSlash) || "/",
    filter: value.slice(lastSlash + 1),
  };
}

export function PathAutocomplete({
  value,
  onChange,
  placeholder,
  disabled,
  className,
  showBrowseButton,
  hostId,
}: PathAutocompleteProps) {
  const [open, setOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(0);
  const [chooserOpen, setChooserOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const blurTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const { queryPath, filter } = getQueryAndFilter(value);
  const { data, isLoading } = useHostFiles(queryPath, hostId);
  const allEntries = data?.entries ?? [];
  const dirNames = allEntries
    .filter((e) => e.is_dir)
    .map((e) => e.name);
  const entries = filter
    ? dirNames.filter((name) =>
        name.toLowerCase().startsWith(filter.toLowerCase()),
      )
    : dirNames;

  const showDropdown = open && isAbsolutePath(value);

  const handleFocus = () => {
    if (isAbsolutePath(value)) {
      setOpen(true);
    }
  };

  const handleBlur = () => {
    blurTimeoutRef.current = setTimeout(() => {
      setOpen(false);
    }, 150);
  };

  const handleSelect = useCallback(
    (entry: string) => {
      const { queryPath: qp } = getQueryAndFilter(value);
      if (isWindowsPath(qp)) {
        const sep = qp.includes("\\") ? "\\" : "/";
        if (qp.endsWith(sep)) {
          onChange(qp + entry);
        } else {
          onChange(qp + sep + entry);
        }
      } else {
        if (qp === "/") {
          onChange("/" + entry);
        } else if (qp.endsWith("/")) {
          onChange(qp + entry);
        } else {
          onChange(qp + "/" + entry);
        }
      }
      setOpen(false);
    },
    [value, onChange],
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!showDropdown || entries.length === 0) return;

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightedIndex((i) => (i + 1) % entries.length);
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightedIndex((i) => (i - 1 + entries.length) % entries.length);
        break;
      case "Enter":
        e.preventDefault();
        if (entries[highlightedIndex]) {
          handleSelect(entries[highlightedIndex]);
        }
        break;
      case "Tab":
        if (entries.length === 1) {
          e.preventDefault();
          handleSelect(entries[0]);
        }
        break;
      case "Escape":
        e.preventDefault();
        setOpen(false);
        break;
    }
  };

  useEffect(() => {
    setHighlightedIndex(0);
  }, [entries.length]);

  useEffect(() => {
    return () => {
      if (blurTimeoutRef.current) {
        clearTimeout(blurTimeoutRef.current);
      }
    };
  }, []);

  return (
    <>
      <div ref={containerRef} className="relative flex gap-1">
        <div className="relative flex-1">
          <Input
            value={value}
            onChange={(e) => {
              onChange(e.target.value);
              if (isAbsolutePath(e.target.value)) {
                setOpen(true);
              }
            }}
            onFocus={handleFocus}
            onBlur={handleBlur}
            onKeyDown={handleKeyDown}
            placeholder={placeholder}
            disabled={disabled}
            className={className}
          />
          {isLoading && (
            <div className="absolute right-2 top-1/2 -translate-y-1/2">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          )}
          {showDropdown && (
            <div className="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-md border bg-popover shadow-md">
              {entries.length === 0 ? (
                <div className="px-3 py-2 text-sm text-muted-foreground">
                  {isLoading ? "加载中..." : "无可用子目录"}
                </div>
              ) : (
                <ul className="py-1">
                  {entries.map((entry, i) => (
                    <li
                      key={entry}
                      className={`cursor-pointer px-3 py-1.5 text-sm truncate ${
                        i === highlightedIndex
                          ? "bg-accent text-accent-foreground"
                          : ""
                      }`}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        handleSelect(entry);
                      }}
                      onMouseEnter={() => setHighlightedIndex(i)}
                    >
                      <span className="font-medium">{entry}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>
        {showBrowseButton && (
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="h-9 w-9 shrink-0"
            disabled={disabled}
            onClick={() => setChooserOpen(true)}
            title="浏览目录"
          >
            <FolderOpen className="h-4 w-4" />
          </Button>
        )}
      </div>
      {showBrowseButton && (
        <DirectoryChooserDialog
          open={chooserOpen}
          onOpenChange={setChooserOpen}
          onSelect={(path) => {
            onChange(path);
            setChooserOpen(false);
          }}
          initialPath={value}
        />
      )}
    </>
  );
}
