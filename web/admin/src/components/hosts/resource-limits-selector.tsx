import { useState } from "react";
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
  memory_limit_mb: number | null;
  cpu_limit: number | null;
  disk_limit_gb: number | null;
}

interface ResourceLimitsSelectorProps {
  value: ResourceLimitsValue;
  onChange: (value: ResourceLimitsValue) => void;
  disabled?: boolean;
}

const MEMORY_PRESETS = [
  { label: "无限制", value: 0 },
  { label: "1 GB", value: 1024 },
  { label: "2 GB", value: 2048 },
  { label: "4 GB (默认)", value: 4096 },
  { label: "8 GB", value: 8192 },
  { label: "16 GB", value: 16384 },
  { label: "自定义...", value: -1 },
] as const;

const CPU_PRESETS = [
  { label: "无限制", value: 0 },
  { label: "0.5 核", value: 0.5 },
  { label: "1 核", value: 1 },
  { label: "2 核 (默认)", value: 2 },
  { label: "4 核", value: 4 },
  { label: "8 核", value: 8 },
  { label: "自定义...", value: -1 },
] as const;

const DISK_PRESETS = [
  { label: "无限制", value: 0 },
  { label: "10 GB", value: 10 },
  { label: "20 GB (默认)", value: 20 },
  { label: "50 GB", value: 50 },
  { label: "100 GB", value: 100 },
  { label: "自定义...", value: -1 },
] as const;

type PresetItem = { label: string; value: number };

function findPreset(presets: readonly PresetItem[], value: number | null): string {
  if (value === null || value === undefined) return "";
  const preset = presets.find((p) => p.value === value);
  if (preset) return String(preset.value);
  return "custom";
}

function getDisplayLabel(presets: readonly PresetItem[], value: number | null, unit: string, defaultLabel: string): string {
  if (value === null) return defaultLabel;
  const preset = presets.find((p) => p.value === value);
  if (preset) return preset.label;
  return `${value} ${unit}`;
}

export function ResourceLimitsSelector({ value, onChange, disabled }: ResourceLimitsSelectorProps) {
  const [customMemory, setCustomMemory] = useState("");
  const [customCPU, setCustomCPU] = useState("");
  const [customDisk, setCustomDisk] = useState("");

  function isInCustomMode(currentValue: number | null, presets: readonly PresetItem[]): boolean {
    if (currentValue === null) return false;
    return !presets.some((p) => p.value === currentValue);
  }

  return (
    <div className="space-y-4">
      {/* 内存选择器 */}
      <div className="space-y-2">
        <Label>内存限制</Label>
        <Select
          value={findPreset(MEMORY_PRESETS, value.memory_limit_mb)}
          onValueChange={(v) => {
            if (v === "custom") {
              setCustomMemory("");
              onChange({ ...value, memory_limit_mb: 0 });
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
              min={0}
              placeholder="自定义 MB"
              disabled={disabled}
              value={value.memory_limit_mb === 0 || value.memory_limit_mb === null ? "" : String(value.memory_limit_mb)}
              onChange={(e) => {
                const v = e.target.value === "" ? 0 : Number(e.target.value);
                if (v >= 0) {
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
            if (v === "custom") {
              setCustomCPU("");
              onChange({ ...value, cpu_limit: 0 });
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
              min={0}
              step={0.5}
              placeholder="自定义核数"
              disabled={disabled}
              value={value.cpu_limit === 0 || value.cpu_limit === null ? "" : String(value.cpu_limit)}
              onChange={(e) => {
                const v = e.target.value === "" ? 0 : Number(e.target.value);
                if (v >= 0) {
                  onChange({ ...value, cpu_limit: v });
                }
              }}
            />
            <span className="text-sm text-muted-foreground shrink-0">核</span>
          </div>
        )}
      </div>

      {/* 磁盘选择器 */}
      <div className="space-y-2">
        <Label>磁盘限制</Label>
        <Select
          value={findPreset(DISK_PRESETS, value.disk_limit_gb)}
          onValueChange={(v) => {
            if (v === "custom") {
              setCustomDisk("");
              onChange({ ...value, disk_limit_gb: 0 });
            } else {
              onChange({ ...value, disk_limit_gb: Number(v) });
            }
          }}
          disabled={disabled}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={getDisplayLabel(DISK_PRESETS, value.disk_limit_gb, "GB", "默认 (20 GB)")}>
              {getDisplayLabel(DISK_PRESETS, value.disk_limit_gb, "GB", "默认 (20 GB)")}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {DISK_PRESETS.map((p) => (
              <SelectItem key={p.value} value={String(p.value)}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {isInCustomMode(value.disk_limit_gb, DISK_PRESETS) && (
          <div className="flex items-center gap-2">
            <Input
              type="number"
              min={0}
              placeholder="自定义 GB"
              disabled={disabled}
              value={value.disk_limit_gb === 0 || value.disk_limit_gb === null ? "" : String(value.disk_limit_gb)}
              onChange={(e) => {
                const v = e.target.value === "" ? 0 : Number(e.target.value);
                if (v >= 0) {
                  onChange({ ...value, disk_limit_gb: v });
                }
              }}
            />
            <span className="text-sm text-muted-foreground shrink-0">GB</span>
          </div>
        )}
      </div>
    </div>
  );
}
