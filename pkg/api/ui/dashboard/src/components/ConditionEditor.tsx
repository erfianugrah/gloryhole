import { useState, useRef, useCallback } from "react";
import { Plus, Trash2, GitBranch, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

// ─── Types ──────────────────────────────────────────────────────────

type LogicalOp = "AND" | "OR";

// Expanded operator set with negated variants — NO standalone NOT groups
type Operator =
  | "=="
  | "!="
  | "contains"
  | "not_contains"
  | "starts_with"
  | "not_starts_with"
  | "ends_with"
  | "not_ends_with"
  | "matches"
  | "not_matches"
  | "in"
  | "not_in"
  | "in_cidr"
  | "not_in_cidr"
  | ">"
  | "<"
  | ">="
  | "<=";

interface Condition {
  id: string;
  field: string;
  operator: Operator;
  value: string;
}

interface ConditionGroup {
  id: string;
  op: LogicalOp;
  conditions: Array<Condition | ConditionGroup>;
}

export type ConditionTree = ConditionGroup;

// ─── Fields & field-aware operators ─────────────────────────────────

const FIELDS = [
  { value: "Domain", label: "Domain" },
  { value: "ClientIP", label: "Client IP" },
  { value: "QueryType", label: "Query Type" },
  { value: "Hour", label: "Hour (0-23)" },
  { value: "Weekday", label: "Weekday (0-6)" },
];

const DOMAIN_OPERATORS: { value: Operator; label: string }[] = [
  { value: "==", label: "equals" },
  { value: "!=", label: "not equals" },
  { value: "contains", label: "contains" },
  { value: "not_contains", label: "not contains" },
  { value: "starts_with", label: "starts with" },
  { value: "not_starts_with", label: "not starts with" },
  { value: "ends_with", label: "ends with" },
  { value: "not_ends_with", label: "not ends with" },
  { value: "matches", label: "matches (regex)" },
  { value: "not_matches", label: "not matches (regex)" },
  { value: "in", label: "in list" },
  { value: "not_in", label: "not in list" },
];

const IP_OPERATORS: { value: Operator; label: string }[] = [
  { value: "==", label: "equals" },
  { value: "!=", label: "not equals" },
  { value: "in_cidr", label: "in CIDR" },
  { value: "not_in_cidr", label: "not in CIDR" },
];

const QTYPE_OPERATORS: { value: Operator; label: string }[] = [
  { value: "==", label: "equals" },
  { value: "!=", label: "not equals" },
  { value: "in", label: "in list" },
  { value: "not_in", label: "not in list" },
];

const NUMBER_OPERATORS: { value: Operator; label: string }[] = [
  { value: "==", label: "equals" },
  { value: "!=", label: "not equals" },
  { value: ">", label: "greater than" },
  { value: "<", label: "less than" },
  { value: ">=", label: "at least" },
  { value: "<=", label: "at most" },
];

function getOperatorsForField(
  field: string,
): { value: Operator; label: string }[] {
  switch (field) {
    case "Domain":
      return DOMAIN_OPERATORS;
    case "ClientIP":
      return IP_OPERATORS;
    case "QueryType":
      return QTYPE_OPERATORS;
    case "Hour":
    case "Weekday":
      return NUMBER_OPERATORS;
    default:
      return DOMAIN_OPERATORS;
  }
}

function getPlaceholder(field: string, operator: Operator): string {
  switch (field) {
    case "Domain":
      if (operator === "matches" || operator === "not_matches")
        return ".*\\.example\\.com";
      if (operator === "in" || operator === "not_in") return "a.com, b.com";
      if (operator === "ends_with" || operator === "not_ends_with")
        return "example.com";
      return "example.com";
    case "ClientIP":
      if (operator === "in_cidr" || operator === "not_in_cidr")
        return "192.168.1.0/24";
      return "192.168.1.100";
    case "QueryType":
      if (operator === "in" || operator === "not_in") return "A, AAAA, CNAME";
      return "A";
    case "Hour":
      return "22";
    case "Weekday":
      return "0";
    default:
      return "value";
  }
}

// ─── Expression generation ──────────────────────────────────────────

/** Escape a value for safe interpolation inside double-quoted expression string literals. */
function escapeExprValue(v: string): string {
  return v.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

function conditionToExpr(c: Condition): string {
  const { field, operator, value } = c;
  const v = escapeExprValue(value);

  // ── Numeric fields (Hour, Weekday) ──
  if (field === "Hour" || field === "Weekday") {
    const op =
      operator === "==" || operator === "!=" || operator === ">" ||
      operator === "<" || operator === ">=" || operator === "<="
        ? operator
        : "==";
    // Numeric values are not quoted; raw value is used as-is.
    return `${field} ${op} ${value}`;
  }

  // ── ClientIP ──
  if (field === "ClientIP") {
    switch (operator) {
      case "==":
        return `IPEquals(ClientIP, "${v}")`;
      case "!=":
        return `!IPEquals(ClientIP, "${v}")`;
      case "in_cidr":
        return `IPInCIDR(ClientIP, "${v}")`;
      case "not_in_cidr":
        return `!IPInCIDR(ClientIP, "${v}")`;
      default:
        return `ClientIP == "${v}"`;
    }
  }

  // ── QueryType ──
  if (field === "QueryType") {
    switch (operator) {
      case "==":
        return `QueryType == "${v}"`;
      case "!=":
        return `QueryType != "${v}"`;
      case "in": {
        const items = value
          .split(",")
          .map((s) => `"${escapeExprValue(s.trim())}"`)
          .join(", ");
        return `QueryTypeIn(QueryType, ${items})`;
      }
      case "not_in": {
        const items = value
          .split(",")
          .map((s) => `"${escapeExprValue(s.trim())}"`)
          .join(", ");
        return `!QueryTypeIn(QueryType, ${items})`;
      }
      default:
        return `QueryType == "${v}"`;
    }
  }

  // ── Domain (default) ──
  switch (operator) {
    case "==":
      return `Domain == "${v}"`;
    case "!=":
      return `Domain != "${v}"`;
    case "contains":
      return `DomainMatches(Domain, "${v}")`;
    case "not_contains":
      return `!DomainMatches(Domain, "${v}")`;
    case "starts_with":
      return `DomainStartsWith(Domain, "${v}")`;
    case "not_starts_with":
      return `!DomainStartsWith(Domain, "${v}")`;
    case "ends_with":
      return `DomainEndsWith(Domain, ".${v}")`;
    case "not_ends_with":
      return `!DomainEndsWith(Domain, ".${v}")`;
    case "matches":
      return `DomainRegex(Domain, "${v}")`;
    case "not_matches":
      return `!DomainRegex(Domain, "${v}")`;
    case "in": {
      const items = value
        .split(",")
        .map((s) => `"${escapeExprValue(s.trim())}"`)
        .join(", ");
      return `Domain in [${items}]`;
    }
    case "not_in": {
      const items = value
        .split(",")
        .map((s) => `"${escapeExprValue(s.trim())}"`)
        .join(", ");
      return `!(Domain in [${items}])`;
    }
    default:
      return `Domain == "${v}"`;
  }
}

function groupToExpr(g: ConditionGroup): string {
  if (g.conditions.length === 0) return "true";

  const parts = g.conditions.map((c) =>
    "op" in c
      ? groupToExpr(c as ConditionGroup)
      : conditionToExpr(c as Condition),
  );

  if (parts.length === 1) return parts[0];
  return `(${parts.join(g.op === "AND" ? " && " : " || ")})`;
}

export function treeToExpression(tree: ConditionTree): string {
  return groupToExpr(tree);
}

// ─── Human-readable summary ─────────────────────────────────────────

const OP_LABELS: Record<string, string> = {
  "==": "=",
  "!=": "!=",
  contains: "contains",
  not_contains: "not contains",
  starts_with: "starts with",
  not_starts_with: "not starts with",
  ends_with: "ends with",
  not_ends_with: "not ends with",
  matches: "~",
  not_matches: "!~",
  in: "in",
  not_in: "not in",
  in_cidr: "in CIDR",
  not_in_cidr: "not in CIDR",
  ">": ">",
  "<": "<",
  ">=": ">=",
  "<=": "<=",
};

function summarizeCondition(c: Condition): string {
  const label = OP_LABELS[c.operator] ?? c.operator;

  if (
    c.operator === "in" ||
    c.operator === "not_in"
  ) {
    const vals = c.value
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    const preview = vals.slice(0, 3).join(", ");
    const suffix = vals.length > 3 ? ", ..." : "";
    return `${c.field} ${label} [${preview}${suffix}]`;
  }

  const isNumeric = c.field === "Hour" || c.field === "Weekday";
  return `${c.field} ${label} ${isNumeric ? c.value : `"${c.value}"`}`;
}

function summarizeGroup(g: ConditionGroup): string {
  if (g.conditions.length === 0) return "(empty)";
  const parts = g.conditions.map((c) =>
    "op" in c
      ? `(${summarizeGroup(c as ConditionGroup)})`
      : summarizeCondition(c as Condition),
  );
  if (parts.length === 1) return parts[0];
  return parts.join(g.op === "AND" ? " AND " : " OR ");
}

export function summarizeTree(tree: ConditionTree): string {
  return summarizeGroup(tree);
}

// ─── Expression parser ──────────────────────────────────────────────

let idCounter = 0;
function uid(): string {
  return `cond_${++idCounter}_${Date.now()}`;
}

/** Find matching closing paren for the opening paren at `start`. */
function findClosingParen(expr: string, start: number): number {
  let depth = 0;
  for (let i = start; i < expr.length; i++) {
    if (expr[i] === "(") depth++;
    else if (expr[i] === ")") {
      depth--;
      if (depth === 0) return i;
    }
  }
  return -1;
}

/** Split string on `delim` only when not inside parentheses or brackets. */
function splitAtDepthZero(expr: string, delim: string): string[] | null {
  const parts: string[] = [];
  let depth = 0;
  let current = 0;

  for (let i = 0; i < expr.length; i++) {
    const ch = expr[i];
    if (ch === "(" || ch === "[") depth++;
    else if (ch === ")" || ch === "]") depth--;
    else if (depth === 0 && expr.slice(i, i + delim.length) === delim) {
      parts.push(expr.slice(current, i));
      i += delim.length - 1;
      current = i + 1;
    }
  }
  parts.push(expr.slice(current));
  return parts.length > 1 ? parts : null;
}

type LeafPattern = {
  regex: RegExp;
  build: (m: RegExpMatchArray) => Condition;
};

const LEAF_PATTERNS: LeafPattern[] = [
  // ── Negated function calls (before positive) ──
  {
    regex: /^!DomainMatches\(Domain,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "not_contains",
      value: m[1],
    }),
  },
  {
    regex: /^!DomainStartsWith\(Domain,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "not_starts_with",
      value: m[1],
    }),
  },
  {
    regex: /^!DomainEndsWith\(Domain,\s*"\.(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "not_ends_with",
      value: m[1],
    }),
  },
  {
    regex: /^!DomainRegex\(Domain,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "not_matches",
      value: m[1],
    }),
  },
  {
    regex: /^!\(Domain\s+in\s+\[(.+?)]\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "not_in",
      value: m[1]
        .replace(/"/g, "")
        .split(",")
        .map((s) => s.trim())
        .join(", "),
    }),
  },
  {
    regex: /^!IPEquals\(ClientIP,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "ClientIP",
      operator: "!=",
      value: m[1],
    }),
  },
  {
    regex: /^!IPInCIDR\(ClientIP,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "ClientIP",
      operator: "not_in_cidr",
      value: m[1],
    }),
  },
  {
    regex: /^!QueryTypeIn\(QueryType,\s*(.+?)\)$/,
    build: (m) => ({
      id: uid(),
      field: "QueryType",
      operator: "not_in",
      value: m[1]
        .replace(/"/g, "")
        .split(",")
        .map((s) => s.trim())
        .join(", "),
    }),
  },

  // ── Positive function calls ──
  {
    regex: /^DomainMatches\(Domain,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "contains",
      value: m[1],
    }),
  },
  {
    regex: /^DomainStartsWith\(Domain,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "starts_with",
      value: m[1],
    }),
  },
  {
    regex: /^DomainEndsWith\(Domain,\s*"\.(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "ends_with",
      value: m[1],
    }),
  },
  {
    regex: /^DomainRegex\(Domain,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "matches",
      value: m[1],
    }),
  },
  {
    regex: /^IPEquals\(ClientIP,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "ClientIP",
      operator: "==",
      value: m[1],
    }),
  },
  {
    regex: /^IPInCIDR\(ClientIP,\s*"(.+?)"\)$/,
    build: (m) => ({
      id: uid(),
      field: "ClientIP",
      operator: "in_cidr",
      value: m[1],
    }),
  },
  {
    regex: /^QueryTypeIn\(QueryType,\s*(.+?)\)$/,
    build: (m) => ({
      id: uid(),
      field: "QueryType",
      operator: "in",
      value: m[1]
        .replace(/"/g, "")
        .split(",")
        .map((s) => s.trim())
        .join(", "),
    }),
  },

  // ── Domain in list ──
  {
    regex: /^Domain\s+in\s+\[(.+?)]$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "in",
      value: m[1]
        .replace(/"/g, "")
        .split(",")
        .map((s) => s.trim())
        .join(", "),
    }),
  },

  // ── String comparisons ──
  {
    regex: /^Domain\s*==\s*"(.+?)"$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "==",
      value: m[1],
    }),
  },
  {
    regex: /^Domain\s*!=\s*"(.+?)"$/,
    build: (m) => ({
      id: uid(),
      field: "Domain",
      operator: "!=",
      value: m[1],
    }),
  },
  {
    regex: /^QueryType\s*==\s*"(.+?)"$/,
    build: (m) => ({
      id: uid(),
      field: "QueryType",
      operator: "==",
      value: m[1],
    }),
  },
  {
    regex: /^QueryType\s*!=\s*"(.+?)"$/,
    build: (m) => ({
      id: uid(),
      field: "QueryType",
      operator: "!=",
      value: m[1],
    }),
  },

  // ── Numeric comparisons ──
  {
    regex: /^(Hour|Weekday)\s*(==|!=|>=|<=|>|<)\s*(\d+)$/,
    build: (m) => ({
      id: uid(),
      field: m[1],
      operator: m[2] as Operator,
      value: m[3],
    }),
  },
];

function parseExpr(expr: string): Condition | ConditionGroup | null {
  expr = expr.trim();
  if (!expr) return null;

  // Strip outer parens if they wrap the entire expression
  if (
    expr.startsWith("(") &&
    findClosingParen(expr, 0) === expr.length - 1
  ) {
    return parseExpr(expr.slice(1, -1));
  }

  // Try leaf patterns first
  for (const { regex, build } of LEAF_PATTERNS) {
    const m = expr.match(regex);
    if (m) return build(m);
  }

  // Try splitting on " && " at depth 0
  const andParts = splitAtDepthZero(expr, " && ");
  if (andParts && andParts.length > 1) {
    const children = andParts.map((p) => parseExpr(p));
    if (children.every((c) => c !== null)) {
      return {
        id: uid(),
        op: "AND",
        conditions: children as Array<Condition | ConditionGroup>,
      };
    }
  }

  // Try splitting on " || " at depth 0
  const orParts = splitAtDepthZero(expr, " || ");
  if (orParts && orParts.length > 1) {
    const children = orParts.map((p) => parseExpr(p));
    if (children.every((c) => c !== null)) {
      return {
        id: uid(),
        op: "OR",
        conditions: children as Array<Condition | ConditionGroup>,
      };
    }
  }

  return null;
}

export function parseExpression(expr: string): ConditionTree | null {
  if (!expr || !expr.trim()) return null;
  const result = parseExpr(expr.trim());
  if (!result) return null;
  // If result is a leaf, wrap in a single-condition AND group
  if (!("op" in result)) {
    return { id: uid(), op: "AND", conditions: [result] };
  }
  return result as ConditionTree;
}

// ─── Helpers ────────────────────────────────────────────────────────

function emptyCondition(): Condition {
  return { id: uid(), field: "Domain", operator: "==", value: "" };
}

export function emptyTree(): ConditionTree {
  return { id: uid(), op: "AND", conditions: [emptyCondition()] };
}

// ─── PillsInput ─────────────────────────────────────────────────────

interface PillsInputProps {
  values: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
}

function PillsInput({ values, onChange, placeholder }: PillsInputProps) {
  const [input, setInput] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const addValue = useCallback(
    (raw: string) => {
      const trimmed = raw.trim();
      if (trimmed && !values.includes(trimmed)) {
        onChange([...values, trimmed]);
      }
    },
    [values, onChange],
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addValue(input);
      setInput("");
    } else if (e.key === "Backspace" && input === "" && values.length > 0) {
      onChange(values.slice(0, -1));
    }
  };

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    e.preventDefault();
    const pasted = e.clipboardData.getData("text");
    const items = pasted
      .split(/[,\n\r]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    const unique = [...new Set([...values, ...items])];
    onChange(unique);
    setInput("");
  };

  const removeValue = (index: number) => {
    onChange(values.filter((_, i) => i !== index));
  };

  return (
    <div
      className="flex flex-wrap items-center gap-1 rounded-md border border-border bg-background px-2 py-1 min-h-[28px] cursor-text flex-1 min-w-0"
      data-testid="pills-input"
      onClick={() => inputRef.current?.focus()}
    >
      {values.map((v, i) => (
        <Badge
          key={`${v}-${i}`}
          className="gap-0.5 px-1.5 py-0 text-[11px] font-data bg-gh-purple/15 text-gh-purple border-gh-purple/25 hover:bg-gh-purple/25"
        >
          {v}
          <button
            type="button"
            className="ml-0.5 hover:text-gh-red transition-colors"
            onClick={(e) => {
              e.stopPropagation();
              removeValue(i);
            }}
          >
            <X className="h-2.5 w-2.5" />
          </button>
        </Badge>
      ))}
      <input
        ref={inputRef}
        type="text"
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onPaste={handlePaste}
        onBlur={() => {
          if (input.trim()) {
            addValue(input);
            setInput("");
          }
        }}
        placeholder={
          values.length === 0 ? (placeholder ?? "Type and press Enter") : ""
        }
        className="flex-1 min-w-[80px] bg-transparent text-xs font-data outline-none placeholder:text-muted-foreground"
      />
    </div>
  );
}

// ─── AND/OR Separator ───────────────────────────────────────────────

interface SeparatorToggleProps {
  op: LogicalOp;
  onToggle: () => void;
}

function SeparatorToggle({ op, onToggle }: SeparatorToggleProps) {
  return (
    <div className="flex items-center gap-2 py-1" data-testid="join-separator">
      <div className="flex-1 border-t border-border" />
      <button
        type="button"
        onClick={onToggle}
        className={cn(
          "text-[10px] font-semibold uppercase tracking-widest rounded px-2 py-0.5",
          "hover:bg-muted/50 cursor-pointer transition-colors",
          op === "AND" ? "text-gh-cyan" : "text-gh-yellow",
        )}
        title={
          op === "AND"
            ? "Click to switch to OR (any match)"
            : "Click to switch to AND (all match)"
        }
        data-testid="join-toggle"
      >
        {op}
      </button>
      <div className="flex-1 border-t border-border" />
    </div>
  );
}

// ─── Top-level editor ───────────────────────────────────────────────

interface ConditionEditorProps {
  tree: ConditionTree;
  onChange: (tree: ConditionTree) => void;
}

export function ConditionEditor({ tree, onChange }: ConditionEditorProps) {
  return (
    <div className="space-y-2" data-testid="condition-editor">
      <span className="text-[11px] font-medium text-muted-foreground">
        Visual Condition Builder
      </span>
      <GroupEditor
        group={tree}
        onChange={(updated) => onChange(updated as ConditionTree)}
        onRemove={undefined}
        depth={0}
      />
    </div>
  );
}

// ─── Group editor (recursive) ───────────────────────────────────────

interface GroupEditorProps {
  group: ConditionGroup;
  onChange: (group: ConditionGroup) => void;
  onRemove: (() => void) | undefined;
  depth: number;
}

function GroupEditor({ group, onChange, onRemove, depth }: GroupEditorProps) {
  const isAnd = group.op === "AND";
  const borderColor = isAnd ? "border-gh-cyan/30" : "border-gh-yellow/30";
  const bgColor = isAnd ? "bg-gh-cyan/5" : "bg-gh-yellow/5";
  const textColor = isAnd ? "text-gh-cyan" : "text-gh-yellow";
  const hint = isAnd ? "every condition must match" : "at least one must match";
  // Neutral styling when group has a single condition at depth 0
  const isNeutral = group.conditions.length <= 1 && depth === 0;

  function updateCondition(
    index: number,
    updated: Condition | ConditionGroup,
  ) {
    const next = [...group.conditions];
    next[index] = updated;
    onChange({ ...group, conditions: next });
  }

  function removeCondition(index: number) {
    const next = group.conditions.filter((_, i) => i !== index);
    if (next.length === 0 && onRemove) {
      onRemove();
    } else {
      onChange({ ...group, conditions: next });
    }
  }

  function addCondition() {
    onChange({
      ...group,
      conditions: [...group.conditions, emptyCondition()],
    });
  }

  function addGroup(op: LogicalOp) {
    const newGroup: ConditionGroup = {
      id: uid(),
      op,
      conditions: [emptyCondition()],
    };
    onChange({ ...group, conditions: [...group.conditions, newGroup] });
  }

  function toggleOp() {
    onChange({ ...group, op: group.op === "AND" ? "OR" : "AND" });
  }

  return (
    <div
      className={cn(
        "rounded-md border pl-3 pr-2 py-2 space-y-2",
        isNeutral ? "border-border" : borderColor,
        isNeutral ? "" : bgColor,
      )}
      data-testid="condition-group"
      data-group-op={group.op}
    >
      {/* Group header */}
      <div className="flex items-center gap-2">
        <GitBranch className={cn("h-3 w-3 shrink-0", textColor)} />

        <Select
          value={group.op}
          onValueChange={(v) =>
            onChange({ ...group, op: v as LogicalOp })
          }
        >
          <SelectTrigger
            className={cn(
              "w-[150px] h-7 text-[11px] font-medium",
              textColor,
            )}
            data-testid="group-op-select"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="AND" className="text-xs">
              Match ALL (AND)
            </SelectItem>
            <SelectItem value="OR" className="text-xs">
              Match ANY (OR)
            </SelectItem>
          </SelectContent>
        </Select>

        <span className="text-[10px] text-muted-foreground">{hint}</span>

        {onRemove && (
          <Button
            variant="ghost"
            size="icon-sm"
            className="ml-auto h-6 w-6 text-muted-foreground hover:text-gh-red"
            onClick={onRemove}
            aria-label="Remove group"
            data-testid="remove-group"
          >
            <Trash2 className="h-3 w-3" />
          </Button>
        )}
      </div>

      {/* Children with AND/OR separators */}
      <div className="space-y-0">
        {group.conditions.map((c, i) => (
          <div key={"op" in c ? c.id : c.id}>
            {i > 0 && (
              <SeparatorToggle op={group.op} onToggle={toggleOp} />
            )}
            {"op" in c ? (
              <GroupEditor
                group={c as ConditionGroup}
                onChange={(updated) => updateCondition(i, updated)}
                onRemove={() => removeCondition(i)}
                depth={depth + 1}
              />
            ) : (
              <ConditionRow
                condition={c as Condition}
                onChange={(updated) => updateCondition(i, updated)}
                onRemove={() => removeCondition(i)}
              />
            )}
          </div>
        ))}
        {group.conditions.length === 0 && (
          <p className="text-xs text-muted-foreground italic pl-1">
            Empty group — add a condition
          </p>
        )}
      </div>

      {/* Add buttons */}
      <div className="flex items-center gap-1.5">
        <Button
          variant="ghost"
          size="sm"
          onClick={addCondition}
          className="h-6 text-[11px] text-muted-foreground hover:text-foreground"
          data-testid="add-condition"
        >
          <Plus className="h-3 w-3 mr-1" />
          Condition
        </Button>
        {depth < 3 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 text-[11px] text-muted-foreground hover:text-foreground"
                data-testid="add-group-trigger"
              >
                <GitBranch className="h-3 w-3 mr-1" />
                Group
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuItem
                onClick={() => addGroup("OR")}
                data-testid="add-or-group"
              >
                <span className="font-medium text-gh-yellow">OR group</span>
                <span className="text-muted-foreground ml-2">
                  at least one
                </span>
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => addGroup("AND")}
                data-testid="add-and-group"
              >
                <span className="font-medium text-gh-cyan">AND group</span>
                <span className="text-muted-foreground ml-2">every one</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </div>
  );
}

// ─── Condition row (leaf) ───────────────────────────────────────────

interface ConditionRowProps {
  condition: Condition;
  onChange: (condition: Condition) => void;
  onRemove: () => void;
}

function ConditionRow({ condition, onChange, onRemove }: ConditionRowProps) {
  const operators = getOperatorsForField(condition.field);
  const isListOp =
    condition.operator === "in" || condition.operator === "not_in";

  function handleFieldChange(newField: string) {
    const newOps = getOperatorsForField(newField);
    const currentValid = newOps.some((op) => op.value === condition.operator);
    onChange({
      ...condition,
      field: newField,
      operator: currentValid ? condition.operator : "==",
      // Clear value when switching between incompatible fields
      value: currentValid ? condition.value : "",
    });
  }

  return (
    <div className="flex items-center gap-1.5" data-testid="condition-row">
      <Select value={condition.field} onValueChange={handleFieldChange}>
        <SelectTrigger
          className="h-7 w-[120px] text-xs shrink-0"
          data-testid="field-select"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {FIELDS.map((f) => (
            <SelectItem key={f.value} value={f.value}>
              {f.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={condition.operator}
        onValueChange={(v) =>
          onChange({ ...condition, operator: v as Operator })
        }
      >
        <SelectTrigger
          className="h-7 w-[150px] text-xs shrink-0"
          data-testid="operator-select"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {operators.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {isListOp ? (
        <PillsInput
          values={
            condition.value
              ? condition.value
                  .split(",")
                  .map((s) => s.trim())
                  .filter(Boolean)
              : []
          }
          onChange={(vals) =>
            onChange({ ...condition, value: vals.join(", ") })
          }
          placeholder={getPlaceholder(condition.field, condition.operator)}
        />
      ) : (
        <Input
          value={condition.value}
          onChange={(e) => onChange({ ...condition, value: e.target.value })}
          placeholder={getPlaceholder(condition.field, condition.operator)}
          className="h-7 text-xs font-data flex-1 min-w-0"
          data-testid="value-input"
        />
      )}

      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onRemove}
        className="h-7 w-7 shrink-0 text-muted-foreground hover:text-gh-red"
        aria-label="Remove condition"
        data-testid="remove-condition"
      >
        <Trash2 className="h-3 w-3" />
      </Button>
    </div>
  );
}
