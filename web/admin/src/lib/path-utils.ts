const WINDOWS_PATH_RE = /^[A-Za-z]:[/\\]/;

export interface Breadcrumb {
  label: string;
  path: string;
}

export function isAbsolutePath(path: string): boolean {
  if (path.startsWith("/")) return true;
  return WINDOWS_PATH_RE.test(path);
}

export function isWindowsPath(path: string): boolean {
  return WINDOWS_PATH_RE.test(path);
}

/**
 * 将 Windows 路径转换为 Docker 可识别的挂载格式。
 * C:\Users\xxx -> /c/Users/xxx
 * Unix 路径原样返回。
 */
export function toDockerMountPath(path: string): string {
  if (!isWindowsPath(path)) return path;
  const drive = path[0].toLowerCase();
  const rest = path.slice(2).replace(/\\/g, "/");
  return `/${drive}${rest}`;
}

export function normalizeSeparators(path: string): string {
  return path.replace(/\\/g, "/");
}

export function buildBreadcrumbs(path: string): Breadcrumb[] {
  if (isWindowsPath(path)) {
    const drive = path.substring(0, 2);
    const rest = normalizeSeparators(path.slice(2)).replace(/^\/+/, "");
    if (!rest) return [{ label: drive, path: drive + "\\" }];
    const parts = rest.split("/").filter(Boolean);
    const items: Breadcrumb[] = [{ label: drive, path: drive + "\\" }];
    let accumulated = drive + "\\";
    for (const part of parts) {
      accumulated += part + "\\";
      items.push({ label: part, path: accumulated });
    }
    return items;
  }

  if (path === "/") return [{ label: "根目录", path: "/" }];
  const parts = path.split("/").filter(Boolean);
  const items: Breadcrumb[] = [{ label: "根目录", path: "/" }];
  let accumulated = "";
  for (const part of parts) {
    accumulated += `/${part}`;
    items.push({ label: part, path: accumulated });
  }
  return items;
}

export function getParentPath(path: string): string {
  if (isWindowsPath(path)) {
    const trimmed = path.replace(/[\\/]+$/, "");
    if (trimmed.length <= 2) return trimmed.endsWith("\\") ? trimmed : trimmed + "\\";
    const lastSep = Math.max(trimmed.lastIndexOf("\\"), trimmed.lastIndexOf("/"));
    if (lastSep <= 1) return trimmed.substring(0, 2) + "\\";
    return trimmed.substring(0, lastSep);
  }
  const trimmed = path.replace(/\/+$/, "");
  if (trimmed === "" || trimmed === "/") return "/";
  const lastSlash = trimmed.lastIndexOf("/");
  if (lastSlash <= 0) return "/";
  return trimmed.substring(0, lastSlash);
}

export function joinPath(parent: string, child: string): string {
  if (isWindowsPath(parent)) {
    const sep = parent.includes("\\") ? "\\" : "/";
    const trimmed = parent.replace(/[\\/]+$/, "");
    return trimmed + sep + child;
  }
  const trimmed = parent.replace(/\/+$/, "");
  return trimmed + "/" + child;
}
