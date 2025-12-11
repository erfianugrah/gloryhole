/**
 * Policy Management Module
 * Handles policy CRUD operations and rule builder UI
 */

let editingPolicyId = null;

const POLICY_BUILDER_FIELDS = [
    {
        key: 'domain',
        label: 'Domain',
        operators: [
            { key: 'equals', label: 'equals', inputType: 'text', build: (value) => `Domain == ${quoteValue(value)}` },
            { key: 'starts_with', label: 'starts with', inputType: 'text', build: (value) => `DomainStartsWith(Domain, ${quoteValue(value)})` },
            { key: 'ends_with', label: 'ends with', inputType: 'text', build: (value) => `DomainEndsWith(Domain, ${quoteValue(value)})` },
            { key: 'contains', label: 'contains', inputType: 'text', build: (value) => `DomainMatches(Domain, ${quoteValue('.*' + escapeRegex(value) + '.*')})` },
            { key: 'matches_regex', label: 'matches regex', inputType: 'text', build: (value) => `DomainMatches(Domain, ${quoteValue(value)})` },
        ],
    },
    {
        key: 'client_ip',
        label: 'Client IP',
        operators: [
            { key: 'equals', label: 'equals', inputType: 'text', build: (value) => `ClientIP == ${quoteValue(value)}` },
            { key: 'in_cidr', label: 'in CIDR', inputType: 'text', placeholder: '10.0.0.0/8', build: (value) => `IPInCIDR(ClientIP, ${quoteValue(value)})` },
        ],
    },
    {
        key: 'query_type',
        label: 'Query Type',
        operators: [
            {
                key: 'equals',
                label: 'equals',
                inputType: 'select',
                options: ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'PTR', 'ANY'],
                build: (value) => `QueryType == ${quoteValue(value.toUpperCase())}`,
            },
        ],
    },
    {
        key: 'hour',
        label: 'Hour',
        operators: [
            { key: 'equals', label: 'equals', inputType: 'number', min: 0, max: 23, build: (value) => `Hour == ${value}` },
            { key: 'after', label: 'after or equal', inputType: 'number', min: 0, max: 23, build: (value) => `Hour >= ${value}` },
            { key: 'before', label: 'before or equal', inputType: 'number', min: 0, max: 23, build: (value) => `Hour <= ${value}` },
        ],
    },
    {
        key: 'response_time',
        label: 'Response Time (ms)',
        operators: [
            { key: 'greater', label: 'greater than', inputType: 'number', min: 0, build: (value) => `ResponseTimeMs >= ${value}` },
            { key: 'less', label: 'less than', inputType: 'number', min: 0, build: (value) => `ResponseTimeMs <= ${value}` },
        ],
    },
];

const builderState = {
    mode: 'builder',
    combinator: '&&',
    conditions: [],
};

export function initPolicyModule() {
    initPolicyBuilder();
    setupModalCloseHandlers();
}

export function showAddPolicyModal() {
    editingPolicyId = null;
    document.getElementById('modal-title').textContent = 'Add Policy';
    document.getElementById('policy-form').reset();
    document.getElementById('policy-id').value = '';
    resetPolicyBuilder();
    setLogicMode('builder');
    document.getElementById('policy-modal').style.display = 'flex';
}

export function showEditPolicyModal(policy) {
    editingPolicyId = policy.id;
    document.getElementById('modal-title').textContent = 'Edit Policy';
    document.getElementById('policy-id').value = policy.id;
    document.getElementById('policy-name').value = policy.name;
    document.getElementById('policy-logic').value = policy.logic;
    document.getElementById('policy-action').value = policy.action;
    document.getElementById('policy-enabled').checked = policy.enabled;
    resetPolicyBuilder();
    const hydrated = restoreBuilderFromLogic(policy.logic);
    setLogicMode(hydrated ? 'builder' : 'expression');
    document.getElementById('policy-modal').style.display = 'flex';
}

export function closePolicyModal() {
    document.getElementById('policy-modal').style.display = 'none';
    editingPolicyId = null;
    resetPolicyBuilder();
}

export async function submitPolicy(event) {
    event.preventDefault();

    const formData = new FormData(event.target);
    const policy = {
        name: formData.get('name'),
        logic: formData.get('logic'),
        action: formData.get('action'),
        enabled: formData.get('enabled') === 'on'
    };

    const policyId = formData.get('id');
    const url = policyId ? `/api/policies/${policyId}` : '/api/policies';
    const method = policyId ? 'PUT' : 'POST';

    try {
        const response = await fetch(url, {
            method: method,
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(policy)
        });

        if (response.ok) {
            closePolicyModal();
            htmx.trigger(document.body, 'policy-updated');
        } else {
            const error = await response.json();
            alert('Error: ' + (error.message || 'Failed to save policy'));
        }
    } catch (error) {
        alert('Error: ' + error.message);
    }
}

export async function deletePolicy(id) {
    if (!confirm('Are you sure you want to delete this policy?')) {
        return;
    }

    try {
        const response = await fetch(`/api/policies/${id}`, {
            method: 'DELETE'
        });

        if (response.ok) {
            htmx.trigger(document.body, 'policy-updated');
        } else {
            const error = await response.json();
            alert('Error: ' + (error.message || 'Failed to delete policy'));
        }
    } catch (error) {
        alert('Error: ' + error.message);
    }
}

export async function togglePolicy(id, enabled) {
    try {
        const response = await fetch(`/api/policies/${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ enabled: enabled })
        });

        if (response.ok) {
            htmx.trigger(document.body, 'policy-updated');
        } else {
            alert('Failed to toggle policy');
        }
    } catch (error) {
        alert('Error: ' + error.message);
    }
}

export async function testPolicyRule() {
    const logic = document.getElementById('policy-logic').value.trim();
    const domain = document.getElementById('policy-test-domain').value.trim();
    const client = document.getElementById('policy-test-client').value.trim();
    const type = document.getElementById('policy-test-type').value;
    const resultEl = document.getElementById('policy-test-result');

    if (!logic || !domain) {
        resultEl.textContent = 'Provide both logic and a sample domain.';
        return;
    }

    resultEl.textContent = 'Testing...';
    try {
        const response = await fetch('/api/policies/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                logic,
                domain,
                client_ip: client,
                query_type: type,
            })
        });
        if (!response.ok) {
            const payload = await response.json();
            throw new Error(payload.message || 'Rule failed to compile');
        }
        const payload = await response.json();
        resultEl.textContent = payload.matched ? 'Matched (rule would trigger).' : 'No match.';
        resultEl.className = payload.matched ? 'text-muted success-text' : 'text-muted';
    } catch (error) {
        resultEl.textContent = error.message || 'Failed to test rule.';
        resultEl.className = 'text-muted danger-text';
    }
}

function initPolicyBuilder() {
    populateFieldOptions();
    document.querySelectorAll('input[name="logic-mode"]').forEach((radio) => {
        radio.addEventListener('change', (event) => setLogicMode(event.target.value));
    });
    document.getElementById('builder-field')?.addEventListener('change', () => {
        updateOperatorOptions();
    });
    document.getElementById('builder-operator')?.addEventListener('change', () => {
        updateValueInput();
    });
    document.getElementById('builder-combinator')?.addEventListener('change', (event) => {
        builderState.combinator = event.target.value === '||' ? '||' : '&&';
        updateLogicFromBuilder();
    });
    updateOperatorOptions();
    setLogicMode('builder');
    renderPolicyConditions();
}

function setupModalCloseHandlers() {
    document.getElementById('policy-modal')?.addEventListener('click', function(e) {
        if (e.target === this) {
            closePolicyModal();
        }
    });
}

function populateFieldOptions() {
    const fieldSelect = document.getElementById('builder-field');
    if (!fieldSelect) return;

    // Clear existing options safely
    while (fieldSelect.firstChild) {
        fieldSelect.removeChild(fieldSelect.firstChild);
    }

    POLICY_BUILDER_FIELDS.forEach((field) => {
        const option = document.createElement('option');
        option.value = field.key;
        option.textContent = field.label;
        fieldSelect.appendChild(option);
    });
}

function getSelectedField() {
    const key = document.getElementById('builder-field')?.value;
    return POLICY_BUILDER_FIELDS.find((field) => field.key === key) || POLICY_BUILDER_FIELDS[0];
}

function getSelectedOperator(field) {
    const key = document.getElementById('builder-operator')?.value;
    return field.operators.find((op) => op.key === key) || field.operators[0];
}

function updateOperatorOptions() {
    const operatorSelect = document.getElementById('builder-operator');
    if (!operatorSelect) return;

    const field = getSelectedField();

    // Clear existing options safely
    while (operatorSelect.firstChild) {
        operatorSelect.removeChild(operatorSelect.firstChild);
    }

    field.operators.forEach((op) => {
        const option = document.createElement('option');
        option.value = op.key;
        option.textContent = op.label;
        operatorSelect.appendChild(option);
    });
    updateValueInput();
}

function updateValueInput() {
    const field = getSelectedField();
    const operator = getSelectedOperator(field);
    const valueInput = document.getElementById('builder-value');
    const valueSelect = document.getElementById('builder-value-select');
    if (!operator || !valueInput || !valueSelect) return;

    const previousText = valueInput.value;
    const previousSelect = valueSelect.value;

    if (operator.inputType === 'select') {
        // Clear existing options safely
        while (valueSelect.firstChild) {
            valueSelect.removeChild(valueSelect.firstChild);
        }

        operator.options.forEach((opt) => {
            const option = document.createElement('option');
            option.value = opt;
            option.textContent = opt;
            valueSelect.appendChild(option);
        });
        valueInput.style.display = 'none';
        valueSelect.style.display = 'inline-flex';
        if (previousSelect && operator.options.includes(previousSelect)) {
            valueSelect.value = previousSelect;
        }
    } else {
        valueSelect.style.display = 'none';
        valueInput.style.display = 'inline-flex';
        valueInput.type = operator.inputType === 'number' ? 'number' : 'text';
        valueInput.placeholder = operator.placeholder || 'Value';
        if (operator.min !== undefined) valueInput.min = operator.min;
        if (operator.max !== undefined) valueInput.max = operator.max;
        if (previousText) {
            valueInput.value = previousText;
        }
    }
}

export function addPolicyCondition() {
    const field = getSelectedField();
    const operator = getSelectedOperator(field);
    const valueInput = document.getElementById('builder-value');
    const valueSelect = document.getElementById('builder-value-select');
    if (!field || !operator) {
        alert('Select a field and operator.');
        return;
    }

    let rawValue = operator.inputType === 'select' ? valueSelect.value : valueInput.value.trim();
    if (rawValue === '') {
        alert('Enter a value for this condition.');
        return;
    }

    let buildValue = rawValue;
    if (operator.inputType === 'number') {
        buildValue = Number(rawValue);
        if (!Number.isFinite(buildValue)) {
            alert('Enter a valid number.');
            return;
        }
    }

    const expression = operator.build(buildValue);
    if (!expression) {
        alert('Unable to build expression for this condition.');
        return;
    }

    builderState.conditions.push({
        expression,
        label: `${field.label} ${operator.label} ${rawValue}`,
    });
    renderPolicyConditions();
    updateLogicFromBuilder();
    valueInput.value = '';
}

function renderPolicyConditions() {
    const container = document.getElementById('builder-conditions');
    if (!container) return;

    // Clear container safely
    while (container.firstChild) {
        container.removeChild(container.firstChild);
    }

    if (builderState.conditions.length === 0) {
        container.classList.add('empty');
        container.textContent = 'No conditions yet. Select a field above to begin.';
        return;
    }
    container.classList.remove('empty');
    builderState.conditions.forEach((condition, index) => {
        const chip = document.createElement('div');
        chip.className = 'condition-chip';
        chip.textContent = condition.label;
        const removeBtn = document.createElement('button');
        removeBtn.type = 'button';
        removeBtn.setAttribute('aria-label', 'Remove condition');
        removeBtn.textContent = '×';
        removeBtn.addEventListener('click', () => removePolicyCondition(index));
        chip.appendChild(removeBtn);
        container.appendChild(chip);
    });
}

function removePolicyCondition(index) {
    builderState.conditions.splice(index, 1);
    renderPolicyConditions();
    updateLogicFromBuilder();
}

function updateLogicFromBuilder() {
    if (builderState.mode !== 'builder') {
        return;
    }
    const textarea = document.getElementById('policy-logic');
    if (!textarea) return;
    if (builderState.conditions.length === 0) {
        textarea.value = '';
        return;
    }
    const joiner = builderState.combinator === '||' ? ' || ' : ' && ';
    textarea.value = builderState.conditions.map((c) => `(${c.expression})`).join(joiner);
}

function resetPolicyBuilder() {
    builderState.conditions = [];
    builderState.combinator = '&&';
    const combinatorSelect = document.getElementById('builder-combinator');
    if (combinatorSelect) combinatorSelect.value = '&&';
    renderPolicyConditions();
    updateLogicFromBuilder();
}

function restoreBuilderFromLogic(logic) {
    if (!logic || typeof logic !== 'string') {
        return false;
    }

    const parts = logic.split(/\s+(\&\&|\|\|)\s+/);
    if (parts.length === 1 && !logic.trim()) {
        return false;
    }

    const expressions = [];
    const joiners = [];
    for (let i = 0; i < parts.length; i++) {
        if (i % 2 === 0) {
            const expr = parts[i].trim().replace(/^\(|\)$/g, '');
            if (expr) expressions.push(expr);
        } else {
            joiners.push(parts[i]);
        }
    }

    if (!expressions.length) {
        return false;
    }

    builderState.conditions = expressions.map((expr) => ({
        expression: expr,
        label: expr,
    }));

    if (joiners.length) {
        builderState.combinator = joiners.some((j) => j === '||') ? '||' : '&&';
        const combinatorSelect = document.getElementById('builder-combinator');
        if (combinatorSelect) combinatorSelect.value = builderState.combinator;
    }

    renderPolicyConditions();
    updateLogicFromBuilder();
    return true;
}

function setLogicMode(mode) {
    builderState.mode = mode;
    const panel = document.getElementById('builder-panel');
    const textarea = document.getElementById('policy-logic');
    if (!panel || !textarea) return;
    if (mode === 'builder') {
        if (builderState.conditions.length === 0 && textarea.value) {
            restoreBuilderFromLogic(textarea.value);
        }
        panel.style.display = 'block';
        textarea.readOnly = true;
        textarea.classList.add('readonly');
        updateLogicFromBuilder();
        document.querySelector('input[name="logic-mode"][value="builder"]').checked = true;
    } else {
        panel.style.display = 'none';
        textarea.readOnly = false;
        textarea.classList.remove('readonly');
        document.querySelector('input[name="logic-mode"][value="expression"]').checked = true;
    }
}

function escapeRegex(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function quoteValue(value) {
    try {
        return JSON.stringify(value);
    } catch (_) {
        return `"${String(value).replace(/\"/g, '\\"')}"`;
    }
}

// Make functions available globally for HTML onclick handlers (temporary until all handlers are migrated)
window.showAddPolicyModal = showAddPolicyModal;
window.showEditPolicyModal = showEditPolicyModal;
window.closePolicyModal = closePolicyModal;
window.submitPolicy = submitPolicy;
window.deletePolicy = deletePolicy;
window.togglePolicy = togglePolicy;
window.testPolicyRule = testPolicyRule;
window.addPolicyCondition = addPolicyCondition;
