import { useState } from "react";
import { Plus, Trash2, GitBranch } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

// ─── Types ──────────────────────────────────────────────────────────

type LogicalOp = "AND" | "OR";
type Operator =
  | "=="
  | "!="
  | "contains"
  | "starts_with"
  | "ends_with"
  | "matches"
  | "in"
  | ">"
  | "<"
  | ">="
  | "<=";

type GroupType = "all" | "any" | "not";

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
  negated?: boolean;
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
  { value: "starts_with", label: "starts with" },
  { value: "ends_with", label: "ends with" },
  { value: "matches", label: "matches (regex)" },
  { value: "in", label: "in list" },
];

const IP_OPERATORS: { value: Operator; label: string }[] = [
  { value: "==", label: "equals" },
  { value: "!=", label: "not equals" },
  { value: "in", label: "in CIDR" },
];

const QTYPE_OPERATORS: { value: Operator; label: string }[] = [
  { value: "==", label: "equals" },
  { value: "!=", label: "not equals" },
  { value: "in", label: "in list" },
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
      if (operator === "matches") return ".*\\.example\\.com";
      if (operator === "in") return "a.com, b.com";
      if (operator === "ends_with") return "example.com";
      return "example.com";
    case "ClientIP":
      if (operator === "in") return "192.168.1.0/24";
      return "192.168.1.100";
    case "QueryType":
      if (operator === "in") return "A, AAAA, CNAME";
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
// Field-aware: numeric fields emit unquoted values, each field maps to
// the correct backend helper function.

function conditionToExpr(c: Condition): string {
  const { field, operator, value } = c;

  // ── Numeric fields (Hour, Weekday) — no quotes, numeric operators ──
  if (field === "Hour" || field === "Weekday") {
    switch (operator) {
      case "==":
        return `${field} == ${value}`;
      case "!=":
        return `${field} != ${value}`;
      case ">":
        return `${field} > ${value}`;
      case "<":
        return `${field} < ${value}`;
      case ">=":
        return `${field} >= ${value}`;
      case "<=":
        return `${field} <= ${value}`;
      default:
        return `${field} == ${value}`;
    }
  }

  // ── ClientIP — uses IP-specific helpers ──
  if (field === "ClientIP") {
    switch (operator) {
      case "==":
        return `IPEquals(ClientIP, "${value}")`;
      case "!=":
        return `!IPEquals(ClientIP, "${value}")`;
      case "in":
        return `IPInCIDR(ClientIP, "${value}")`;
      default:
        return `ClientIP == "${value}"`;
    }
  }

  // ── QueryType — string comparison + QueryTypeIn for lists ──
  if (field === "QueryType") {
    switch (operator) {
      case "==":
        return `QueryType == "${value}"`;
      case "!=":
        return `QueryType != "${value}"`;
      case "in":
        return `QueryTypeIn(QueryType, ${value
          .split(",")
          .map((v) => `"${v.trim()}"`)
          .join(", ")})`;
      default:
        return `QueryType == "${value}"`;
    }
  }

  // ── Domain (default) — full set of domain helpers ──
  switch (operator) {
    case "==":
      return `Domain == "${value}"`;
    case "!=":
      return `Domain != "${value}"`;
    case "contains":
      return `DomainMatches(Domain, "${value}")`;
    case "starts_with":
      return `DomainStartsWith(Domain, "${value}")`;
    case "ends_with":
      return `DomainEndsWith(Domain, ".${value}")`;
    case "matches":
      return `DomainRegex(Domain, "${value}")`;
    case "in":
      return `Domain in [${value
        .split(",")
        .map((v) => `"${v.trim()}"`)
        .join(", ")}]`;
    default:
      return `Domain == "${value}"`;
  }
}

function groupToExpr(g: ConditionGroup): string {
  if (g.conditions.length === 0) return "true";

  const parts = g.conditions.map((c) =>
    "op" in c
      ? groupToExpr(c as ConditionGroup)
      : conditionToExpr(c as Condition),
  );

  const joined =
    parts.length === 1
      ? parts[0]
      : `(${parts.join(g.op === "AND" ? " && " : " || ")})`;

  return g.negated ? `!(${joined})` : joined;
}

export function treeToExpression(tree: ConditionTree): string {
  return groupToExpr(tree);
}

// ─── Group type helpers ─────────────────────────────────────────────

function getGroupType(g: ConditionGroup): GroupType {
  if (g.negated) return "not";
  return g.op === "AND" ? "all" : "any";
}

const GROUP_STYLES: Record<
  GroupType,
  { border: string; bg: string; text: string; label: string; hint: string }
> = {
  all: {
    border: "border-gh-cyan/30",
    bg: "bg-gh-cyan/5",
    text: "text-gh-cyan",
    label: "Match ALL (AND)",
    hint: "every condition must match",
  },
  any: {
    border: "border-gh-yellow/30",
    bg: "bg-gh-yellow/5",
    text: "text-gh-yellow",
    label: "Match ANY (OR)",
    hint: "at least one must match",
  },
  not: {
    border: "border-gh-red/30",
    bg: "bg-gh-red/5",
    text: "text-gh-red",
    label: "NOT",
    hint: "inverts the result",
  },
};

// ─── Helpers ────────────────────────────────────────────────────────

let idCounter = 0;
function uid(): string {
  return `cond_${++idCounter}_${Date.now()}`;
}

function emptyCondition(): Condition {
  return { id: uid(), field: "Domain", operator: "==", value: "" };
}

export function emptyTree(): ConditionTree {
  return { id: uid(), op: "AND", conditions: [emptyCondition()] };
}

// ─── Top-level editor ───────────────────────────────────────────────

interface ConditionEditorProps {
  tree: ConditionTree;
  onChange: (tree: ConditionTree) => void;
}

export function ConditionEditor({ tree, onChange }: ConditionEditorProps) {
  return (
    <div className="space-y-2">
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
  const groupType = getGroupType(group);
  const style = GROUP_STYLES[groupType];

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
    onChange({ ...group, conditions: next });
  }

  function addCondition() {
    onChange({
      ...group,
      conditions: [...group.conditions, emptyCondition()],
    });
  }

  function addGroup() {
    const newGroup: ConditionGroup = {
      id: uid(),
      op: "AND",
      conditions: [emptyCondition()],
    };
    onChange({ ...group, conditions: [...group.conditions, newGroup] });
  }

  function switchType(newType: GroupType) {
    switch (newType) {
      case "all":
        onChange({ ...group, op: "AND", negated: false });
        break;
      case "any":
        onChange({ ...group, op: "OR", negated: false });
        break;
      case "not":
        onChange({ ...group, negated: true });
        break;
    }
  }

  return (
    <div
      className={cn(
        "rounded-md border pl-3 pr-2 py-2 space-y-2",
        style.border,
        style.bg,
      )}
    >
      {/* Group header */}
      <div className="flex items-center gap-2">
        <GitBranch className={cn("h-3 w-3 shrink-0", style.text)} />

        <Select
          value={groupType}
          onValueChange={(v) => switchType(v as GroupType)}
        >
          <SelectTrigger
            className={cn("w-[150px] h-7 text-[11px] font-medium", style.text)}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all" className="text-xs">
              Match ALL (AND)
            </SelectItem>
            <SelectItem value="any" className="text-xs">
              Match ANY (OR)
            </SelectItem>
            <SelectItem value="not" className="text-xs">
              NOT
            </SelectItem>
          </SelectContent>
        </Select>

        <span className="text-[10px] text-muted-foreground">{style.hint}</span>

        {onRemove && (
          <Button
            variant="ghost"
            size="icon-sm"
            className="ml-auto h-6 w-6 text-muted-foreground hover:text-gh-red"
            onClick={onRemove}
            aria-label="Remove group"
          >
            <Trash2 className="h-3 w-3" />
          </Button>
        )}
      </div>

      {/* Children */}
      <div className="space-y-2">
        {group.conditions.map((c, i) =>
          "op" in c ? (
            <GroupEditor
              key={c.id}
              group={c as ConditionGroup}
              onChange={(updated) => updateCondition(i, updated)}
              onRemove={() => removeCondition(i)}
              depth={depth + 1}
            />
          ) : (
            <ConditionRow
              key={c.id}
              condition={c as Condition}
              onChange={(updated) => updateCondition(i, updated)}
              onRemove={() => removeCondition(i)}
            />
          ),
        )}
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
        >
          <Plus className="h-3 w-3 mr-1" />
          Condition
        </Button>
        {depth < 3 && (
          <Button
            variant="ghost"
            size="sm"
            onClick={addGroup}
            className="h-6 text-[11px] text-muted-foreground hover:text-foreground"
          >
            <GitBranch className="h-3 w-3 mr-1" />
            Group
          </Button>
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

  function handleFieldChange(newField: string) {
    const newOps = getOperatorsForField(newField);
    const currentValid = newOps.some((op) => op.value === condition.operator);
    onChange({
      ...condition,
      field: newField,
      // Reset to "==" if current operator isn't available for new field
      operator: currentValid ? condition.operator : "==",
    });
  }

  return (
    <div className="flex items-center gap-1.5">
      <Select value={condition.field} onValueChange={handleFieldChange}>
        <SelectTrigger className="h-7 w-[120px] text-xs shrink-0">
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
        <SelectTrigger className="h-7 w-[130px] text-xs shrink-0">
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

      <Input
        value={condition.value}
        onChange={(e) => onChange({ ...condition, value: e.target.value })}
        placeholder={getPlaceholder(condition.field, condition.operator)}
        className="h-7 text-xs font-data flex-1 min-w-0"
      />

      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onRemove}
        className="h-7 w-7 shrink-0 text-muted-foreground hover:text-gh-red"
        aria-label="Remove condition"
      >
        <Trash2 className="h-3 w-3" />
      </Button>
    </div>
  );
}
