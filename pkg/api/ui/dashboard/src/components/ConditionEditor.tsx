import { useState } from "react";
import { Plus, Trash2, ChevronDown, ChevronRight } from "lucide-react";
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

// --- Types ---

type LogicalOp = "AND" | "OR";
type Operator = "==" | "!=" | "contains" | "starts_with" | "ends_with" | "matches" | "in";

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

// --- Fields & operators ---

const FIELDS = [
  { value: "Domain", label: "Domain" },
  { value: "ClientIP", label: "Client IP" },
  { value: "QueryType", label: "Query Type" },
  { value: "Hour", label: "Hour (0-23)" },
  { value: "Weekday", label: "Weekday (0-6)" },
];

const OPERATORS: { value: Operator; label: string }[] = [
  { value: "==", label: "equals" },
  { value: "!=", label: "not equals" },
  { value: "contains", label: "contains" },
  { value: "starts_with", label: "starts with" },
  { value: "ends_with", label: "ends with" },
  { value: "matches", label: "matches (regex)" },
  { value: "in", label: "in list" },
];

// --- Expression generation ---

function conditionToExpr(c: Condition): string {
  const { field, operator, value } = c;
  switch (operator) {
    case "==": return `${field} == "${value}"`;
    case "!=": return `${field} != "${value}"`;
    case "contains": return `DomainMatches(${field}, "${value}")`;
    case "starts_with": return `DomainStartsWith(${field}, "${value}")`;
    case "ends_with": return `DomainEndsWith(${field}, ".${value}")`;
    case "matches": return `DomainMatches(${field}, "${value}")`;
    case "in":
      if (field === "ClientIP") return `IPInCIDR(ClientIP, "${value}")`;
      return `${field} in [${value.split(",").map(v => `"${v.trim()}"`).join(", ")}]`;
    default: return `${field} == "${value}"`;
  }
}

function groupToExpr(g: ConditionGroup): string {
  if (g.conditions.length === 0) return "true";

  const parts = g.conditions.map((c) =>
    "op" in c ? groupToExpr(c as ConditionGroup) : conditionToExpr(c as Condition)
  );

  const joined = parts.length === 1
    ? parts[0]
    : `(${parts.join(g.op === "AND" ? " && " : " || ")})`;

  return g.negated ? `!(${joined})` : joined;
}

export function treeToExpression(tree: ConditionTree): string {
  return groupToExpr(tree);
}

// --- Helpers ---

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

// --- Components ---

interface ConditionEditorProps {
  tree: ConditionTree;
  onChange: (tree: ConditionTree) => void;
}

export function ConditionEditor({ tree, onChange }: ConditionEditorProps) {
  return (
    <div className="space-y-2 rounded-lg border border-border p-3 bg-muted/30">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">Visual Condition Builder</span>
      </div>
      <GroupEditor
        group={tree}
        onChange={(updated) => onChange(updated as ConditionTree)}
        onRemove={undefined}
        depth={0}
      />
    </div>
  );
}

interface GroupEditorProps {
  group: ConditionGroup;
  onChange: (group: ConditionGroup) => void;
  onRemove: (() => void) | undefined;
  depth: number;
}

function GroupEditor({ group, onChange, onRemove, depth }: GroupEditorProps) {
  const [collapsed, setCollapsed] = useState(false);

  function updateCondition(index: number, updated: Condition | ConditionGroup) {
    const next = [...group.conditions];
    next[index] = updated;
    onChange({ ...group, conditions: next });
  }

  function removeCondition(index: number) {
    const next = group.conditions.filter((_, i) => i !== index);
    onChange({ ...group, conditions: next });
  }

  function addCondition() {
    onChange({ ...group, conditions: [...group.conditions, emptyCondition()] });
  }

  function addGroup() {
    const newGroup: ConditionGroup = { id: uid(), op: "AND", conditions: [emptyCondition()] };
    onChange({ ...group, conditions: [...group.conditions, newGroup] });
  }

  return (
    <div className={cn("space-y-2", depth > 0 && "border-l-2 border-border pl-3 ml-1")}>
      <div className="flex items-center gap-2">
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="text-muted-foreground hover:text-foreground"
          aria-label={collapsed ? "Expand group" : "Collapse group"}
        >
          {collapsed ? <ChevronRight className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
        </button>

        <Select
          value={group.op}
          onValueChange={(v) => onChange({ ...group, op: v as LogicalOp })}
        >
          <SelectTrigger className="h-6 w-[70px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="AND">AND</SelectItem>
            <SelectItem value="OR">OR</SelectItem>
          </SelectContent>
        </Select>

        <button
          onClick={() => onChange({ ...group, negated: !group.negated })}
          className={cn(
            "text-[10px] font-mono px-1.5 py-0.5 rounded border",
            group.negated
              ? "bg-destructive/10 border-destructive/30 text-destructive"
              : "bg-muted border-border text-muted-foreground hover:text-foreground"
          )}
        >
          NOT
        </button>

        <div className="flex-1" />

        <Button variant="ghost" size="sm" onClick={addCondition} className="h-6 text-xs px-2">
          <Plus className="h-3 w-3 mr-0.5" />
          Rule
        </Button>
        <Button variant="ghost" size="sm" onClick={addGroup} className="h-6 text-xs px-2">
          <Plus className="h-3 w-3 mr-0.5" />
          Group
        </Button>

        {onRemove && (
          <Button variant="ghost" size="icon-sm" onClick={onRemove} className="h-6 w-6 text-destructive" aria-label="Remove group">
            <Trash2 className="h-3 w-3" />
          </Button>
        )}
      </div>

      {!collapsed && (
        <div className="space-y-1.5">
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
            )
          )}
          {group.conditions.length === 0 && (
            <p className="text-xs text-muted-foreground italic pl-2">Empty group — add a rule or nested group</p>
          )}
        </div>
      )}
    </div>
  );
}

interface ConditionRowProps {
  condition: Condition;
  onChange: (condition: Condition) => void;
  onRemove: () => void;
}

function ConditionRow({ condition, onChange, onRemove }: ConditionRowProps) {
  return (
    <div className="flex items-center gap-1.5">
      <Select
        value={condition.field}
        onValueChange={(v) => onChange({ ...condition, field: v })}
      >
        <SelectTrigger className="h-7 w-[120px] text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {FIELDS.map((f) => (
            <SelectItem key={f.value} value={f.value}>{f.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={condition.operator}
        onValueChange={(v) => onChange({ ...condition, operator: v as Operator })}
      >
        <SelectTrigger className="h-7 w-[120px] text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {OPERATORS.map((o) => (
            <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Input
        value={condition.value}
        onChange={(e) => onChange({ ...condition, value: e.target.value })}
        placeholder="value"
        className="h-7 text-xs font-data flex-1"
      />

      <Button variant="ghost" size="icon-sm" onClick={onRemove} className="h-7 w-7 text-destructive" aria-label="Remove condition">
        <Trash2 className="h-3 w-3" />
      </Button>
    </div>
  );
}
