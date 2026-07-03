import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export interface ResourceLimitsValue {
  pids_limit: number | null;
  memory_limit_mb: number | null;
  cpu_limit: number | null;
}

interface ResourceLimitsSelectorProps {
  value: ResourceLimitsValue;
  onChange: (value: ResourceLimitsValue) => void;
  disabled?: boolean;
}

const PIDS_PRESETS = [
  { label: "512", value: 512 },
  { label: "1024 (默认)", value: 1024 },
  { label: "2048", value: 2048 },
  { label: "4096", value: 4096 },
  { label: "自定义...", value: -1 },
] as const;

const MEMORY_PRESETS = [
  { label: "1 GB", value: 1024 },
  { label: "2 GB", value: 2048 },
  { label: "4 GB (默认)", value: 4096 },
  { label: "8 GB", value: 8192 },
  { label: "16 GB", value: 16384 },
  { label: "自定义...", value: -1 },
] as const;

const CPU_PRESETS = [
  { label: "0.5 核", value: 0.5 },
  { label: "1 核", value: 1 },
  { label: "2 核 (默认)", value: 2 },
  { label: "4 核", value: 4 },
  { label: "8 核", value: 8 },
  { label: "自定义...", value: -1 },
] as const;

type PresetItem = { label: string; value: number };

function findPreset(presets: readonly PresetItem[], value: number | null): string {
  if (value === null || value === undefined || value === 0) return "";
  const preset = presets.find((p) => p.value !== -1 && p.value === value);
  if (preset) return String(preset.value);
  return "-1";
}

function getDisplayLabel(presets: readonly PresetItem[], value: number | null, unit: string, defaultLabel: string): string {
  if (value === null || value === 0) return defaultLabel;
  const preset = presets.find((p) => p.value === value);
  if (preset) return preset.label;
  return unit ? `${value} ${unit}` : String(value);
}

export function ResourceLimitsSelector({ value, onChange, disabled }: ResourceLimitsSelectorProps) {
  function isInCustomMode(currentValue: number | null, presets: readonly PresetItem[]): boolean {
    if (currentValue === null) return false;
    return !presets.some((p) => p.value !== -1 && p.value === currentValue);
  }

  return (
    <div className="space-y-4">
      {/* 进程数选择器 */}
      <div className="space-y-2">
        <Label>进程数限制</Label>
        <Select
          value={findPreset(PIDS_PRESETS, value.pids_limit)}
          onValueChange={(v) => {
            if (v === "-1") {
              onChange({ ...value, pids_limit: 1536 });
            } else {
              onChange({ ...value, pids_limit: Number(v) });
            }
          }}
          disabled={disabled}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={getDisplayLabel(PIDS_PRESETS, value.pids_limit, "", "默认 (1024)")}>
              {getDisplayLabel(PIDS_PRESETS, value.pids_limit, "", "默认 (1024)")}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {PIDS_PRESETS.map((p) => (
              <SelectItem key={p.value} value={String(p.value)}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {isInCustomMode(value.pids_limit, PIDS_PRESETS) && (
          <div className="flex items-center gap-2">
            <Input
              type="number"
                min={64}
                step={1}
                placeholder="自定义进程数"
                disabled={disabled}
                value={value.pids_limit === 0 || value.pids_limit === null ? "" : String(value.pids_limit)}
                onChange={(e) => {
                  const v = e.target.value === "" ? null : Number(e.target.value);
                  if (v === null || (Number.isInteger(v) && v >= 64)) {
                    onChange({ ...value, pids_limit: v });
                  }
                }}
            />
            <span className="text-sm text-muted-foreground shrink-0">个</span>
          </div>
        )}
      </div>

      {/* 内存选择器 */}
      <div className="space-y-2">
        <Label>内存限制</Label>
        <Select
          value={findPreset(MEMORY_PRESETS, value.memory_limit_mb)}
          onValueChange={(v) => {
            if (v === "-1") {
              onChange({ ...value, memory_limit_mb: 6144 });
            } else {
              onChange({ ...value, memory_limit_mb: Number(v) });
            }
          }}
          disabled={disabled}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={getDisplayLabel(MEMORY_PRESETS, value.memory_limit_mb, "MB", "默认 (4 GB)")}>
              {getDisplayLabel(MEMORY_PRESETS, value.memory_limit_mb, "MB", "默认 (4 GB)")}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {MEMORY_PRESETS.map((p) => (
              <SelectItem key={p.value} value={String(p.value)}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {isInCustomMode(value.memory_limit_mb, MEMORY_PRESETS) && (
          <div className="flex items-center gap-2">
            <Input
              type="number"
                min={128}
                placeholder="自定义 MB"
                disabled={disabled}
                value={value.memory_limit_mb === 0 || value.memory_limit_mb === null ? "" : String(value.memory_limit_mb)}
                onChange={(e) => {
                  const v = e.target.value === "" ? null : Number(e.target.value);
                  if (v === null || v >= 128) {
                    onChange({ ...value, memory_limit_mb: v });
                  }
                }}
            />
            <span className="text-sm text-muted-foreground shrink-0">MB</span>
          </div>
        )}
      </div>

      {/* CPU 选择器 */}
      <div className="space-y-2">
        <Label>CPU 限制</Label>
        <Select
          value={findPreset(CPU_PRESETS, value.cpu_limit)}
          onValueChange={(v) => {
            if (v === "-1") {
              onChange({ ...value, cpu_limit: 3 });
            } else {
              onChange({ ...value, cpu_limit: Number(v) });
            }
          }}
          disabled={disabled}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={getDisplayLabel(CPU_PRESETS, value.cpu_limit, "核", "默认 (2 核)")}>
              {getDisplayLabel(CPU_PRESETS, value.cpu_limit, "核", "默认 (2 核)")}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {CPU_PRESETS.map((p) => (
              <SelectItem key={p.value} value={String(p.value)}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {isInCustomMode(value.cpu_limit, CPU_PRESETS) && (
          <div className="flex items-center gap-2">
            <Input
              type="number"
                min={0.1}
                step={0.5}
                placeholder="自定义核数"
                disabled={disabled}
                value={value.cpu_limit === 0 || value.cpu_limit === null ? "" : String(value.cpu_limit)}
                onChange={(e) => {
                  const v = e.target.value === "" ? null : Number(e.target.value);
                  if (v === null || v >= 0.1) {
                    onChange({ ...value, cpu_limit: v });
                  }
                }}
            />
            <span className="text-sm text-muted-foreground shrink-0">核</span>
          </div>
        )}
      </div>
    </div>
  );
}
