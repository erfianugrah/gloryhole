/**
 * Clients Page Module
 * Manages client list, groups, pagination, and sorting
 */

import { fetchJSON, putJSON, postJSON, deleteJSON } from '../utils/api.js';

// DOM elements
let clientsContainer;
let searchInput;
let limitInput;
let offsetInput;
let pageLabel;
let paginationButtons;
let modal;
let groupModal;
let groupList;
let groupTemplate;
let clientGroupSelect;

// State
let searchTimer = null;
let paginationMeta = {
    limit: 1,
    offset: 0,
    has_more: false
};

/**
 * Initialize clients page
 */
export function initializeClients() {
    // Get DOM elements
    clientsContainer = document.getElementById('clients-table-container');
    searchInput = document.getElementById('client-search');
    limitInput = document.getElementById('clients-limit');
    offsetInput = document.getElementById('clients-offset');
    pageLabel = document.getElementById('clients-current-page');
    paginationButtons = document.querySelectorAll('[data-clients-page-shift]');
    modal = document.getElementById('client-modal');
    groupModal = document.getElementById('group-modal');
    groupList = document.getElementById('group-list');
    groupTemplate = document.getElementById('group-row-template');
    clientGroupSelect = document.getElementById('client-group');

    // Initialize pagination meta from DOM
    paginationMeta = {
        limit: Number(limitInput?.value) || 1,
        offset: Number(offsetInput?.value) || 0,
        has_more: clientsContainer?.dataset.clientsHasMore !== 'false'
    };

    // Set up event listeners
    setupEventListeners();

    // Initial setup
    updatePaginationControls();
    refreshGroups();
    attachClientSorting();
}

/**
 * Set up all event listeners
 */
function setupEventListeners() {
    // Client edit button clicks
    document.addEventListener('click', (event) => {
        const target = event.target;
        if (!(target instanceof HTMLElement)) {
            return;
        }
        const trigger = target.closest('[data-edit-client]');
        if (trigger) {
            const row = trigger.closest('[data-client-row]');
            if (row) {
                openClientModal(row);
            }
        }
    });

    // Modal close buttons
    document.querySelectorAll('[data-close-modal]').forEach((btn) => {
        btn.addEventListener('click', () => {
            closeModal(btn.closest('.modal'));
        });
    });

    // Open group modal button
    document.querySelector('[data-open-group-modal]')?.addEventListener('click', () => {
        openModal(groupModal);
        refreshGroups();
    });

    // Client form submission
    document.getElementById('client-form')?.addEventListener('submit', handleClientFormSubmit);

    // Group list actions (edit/delete)
    groupList?.addEventListener('click', handleGroupListClick);

    // Group form submission
    document.getElementById('group-form')?.addEventListener('submit', handleGroupFormSubmit);

    // Group form reset
    document.querySelector('[data-reset-group]')?.addEventListener('click', () => {
        document.getElementById('group-form')?.reset();
        const originalName = document.getElementById('group-original-name');
        if (originalName) originalName.value = '';
    });

    // Pagination buttons
    paginationButtons.forEach((button) => {
        button.addEventListener('click', () => handlePaginationClick(button));
    });

    // Search input
    searchInput?.addEventListener('input', scheduleSearchRefresh);

    // Custom event for pagination metadata updates
    document.addEventListener('clients-page-meta', handlePageMetaUpdate);
}

/**
 * Open a modal
 */
function openModal(target) {
    if (target) {
        target.style.display = 'flex';
    }
}

/**
 * Close a modal
 */
function closeModal(target) {
    if (target) {
        target.style.display = 'none';
    }
}

/**
 * Trigger HTMX refresh for clients container
 */
function triggerClientsRefresh(eventName) {
    if (clientsContainer && window.htmx) {
        window.htmx.trigger(clientsContainer, eventName || 'refresh');
    }
}

/**
 * Update pagination controls (buttons, page label)
 */
function updatePaginationControls(meta) {
    if (!meta) {
        meta = paginationMeta;
    }
    const limit = Number(meta.limit ?? paginationMeta.limit ?? 1);
    const offset = Number(meta.offset ?? paginationMeta.offset ?? 0);
    const hasMore = meta.has_more !== undefined ? meta.has_more : paginationMeta.has_more;
    paginationMeta = { limit, offset, has_more: hasMore };

    if (pageLabel) {
        pageLabel.textContent = Math.floor(offset / limit) + 1;
    }

    const prevBtn = document.querySelector('[data-clients-page-shift="-1"]');
    const nextBtn = document.querySelector('[data-clients-page-shift="1"]');

    if (prevBtn) {
        prevBtn.disabled = offset === 0;
    }
    if (nextBtn) {
        nextBtn.disabled = hasMore === false;
    }

    if (clientsContainer) {
        clientsContainer.dataset.clientsLimit = String(limit);
        clientsContainer.dataset.clientsOffset = String(offset);
        clientsContainer.dataset.clientsHasMore = hasMore === false ? 'false' : 'true';
    }
}

/**
 * Refresh groups list and dropdown
 */
async function refreshGroups() {
    try {
        const data = await fetchJSON('/api/client-groups');
        const groups = data.groups || [];

        // Clear existing groups
        while (groupList.firstChild) {
            groupList.removeChild(groupList.firstChild);
        }

        // Reset dropdown
        clientGroupSelect.textContent = '';
        const unassignedOption = document.createElement('option');
        unassignedOption.value = '';
        unassignedOption.textContent = 'Unassigned';
        clientGroupSelect.appendChild(unassignedOption);

        // Populate groups
        groups.forEach((group) => {
            // Add to dropdown
            const option = document.createElement('option');
            option.value = group.name;
            option.textContent = group.name;
            clientGroupSelect.appendChild(option);

            // Add to group list
            const clone = groupTemplate.content.cloneNode(true);
            const chip = clone.querySelector('.group-chip');
            chip.textContent = group.name;
            chip.style.setProperty('--chip-color', group.color || '#3b82f6');
            clone.querySelector('.group-name').textContent = group.name;
            clone.querySelector('.group-description').textContent = group.description || 'No description';
            clone.querySelector('[data-edit-group]').dataset.group = group.name;
            clone.querySelector('[data-delete-group]').dataset.group = group.name;
            clone.querySelector('[data-delete-group]').dataset.color = group.color || '#3b82f6';
            clone.querySelector('[data-delete-group]').dataset.description = group.description || '';
            clone.querySelector('[data-edit-group]').dataset.color = group.color || '#3b82f6';
            clone.querySelector('[data-edit-group]').dataset.description = group.description || '';
            groupList.appendChild(clone);
        });
    } catch (err) {
        console.error('Failed to load groups', err);
    }
}

/**
 * Open client edit modal
 */
function openClientModal(row) {
    const ip = row.dataset.client;
    const name = row.dataset.name;
    const notes = row.dataset.notes || '';
    const group = row.dataset.group || '';

    document.getElementById('client-ip').value = ip;
    document.getElementById('client-name').value = name === ip ? '' : name;
    document.getElementById('client-notes').value = notes;
    clientGroupSelect.value = group;

    openModal(modal);
}

/**
 * Handle client form submission
 */
async function handleClientFormSubmit(event) {
    event.preventDefault();
    const ip = document.getElementById('client-ip').value;
    const payload = {
        display_name: document.getElementById('client-name').value,
        group_name: clientGroupSelect.value,
        notes: document.getElementById('client-notes').value,
    };

    try {
        await putJSON(`/api/clients/${encodeURIComponent(ip)}`, payload);
        closeModal(modal);
        triggerClientsRefresh('refresh');
    } catch (err) {
        alert(err.message || 'Failed to update client');
    }
}

/**
 * Handle group list clicks (edit/delete buttons)
 */
async function handleGroupListClick(event) {
    const target = event.target;
    if (!(target instanceof HTMLElement)) return;

    const name = target.dataset.group;

    if (target.dataset.editGroup) {
        document.getElementById('group-original-name').value = name;
        document.getElementById('group-name').value = name || '';
        document.getElementById('group-description').value = target.dataset.description || '';
        document.getElementById('group-color').value = target.dataset.color || '#3b82f6';
    }

    if (target.dataset.deleteGroup && name) {
        if (confirm(`Delete group "${name}"? Clients will be unassigned.`)) {
            try {
                await deleteJSON(`/api/client-groups/${encodeURIComponent(name)}`);
                refreshGroups();
                triggerClientsRefresh('refresh');
            } catch (err) {
                alert(err.message || 'Failed to delete group');
            }
        }
    }
}

/**
 * Handle group form submission
 */
async function handleGroupFormSubmit(event) {
    event.preventDefault();
    const original = document.getElementById('group-original-name').value;
    const payload = {
        name: document.getElementById('group-name').value,
        description: document.getElementById('group-description').value,
        color: document.getElementById('group-color').value,
    };

    const url = original
        ? `/api/client-groups/${encodeURIComponent(original)}`
        : '/api/client-groups';
    const method = original ? 'PUT' : 'POST';

    try {
        if (method === 'PUT') {
            await putJSON(url, payload);
        } else {
            await postJSON(url, payload);
        }
        document.getElementById('group-form')?.reset();
        const originalName = document.getElementById('group-original-name');
        if (originalName) originalName.value = '';
        refreshGroups();
    } catch (err) {
        alert(err.message || 'Failed to save group');
    }
}

/**
 * Handle pagination button clicks
 */
function handlePaginationClick(button) {
    const delta = Number(button.dataset.clientsPageShift) || 0;
    const limit = Number(limitInput?.value) || 1;
    const currentOffset = Number(offsetInput?.value) || 0;

    if (delta < 0 && currentOffset === 0) {
        return;
    }
    if (delta > 0 && paginationMeta.has_more === false) {
        return;
    }

    const nextOffset = Math.max(0, currentOffset + delta * limit);
    if (offsetInput) offsetInput.value = String(nextOffset);
    updatePaginationControls({ limit, offset: nextOffset, has_more: true });
    triggerClientsRefresh('refresh');
}

/**
 * Schedule search refresh with debounce
 */
function scheduleSearchRefresh() {
    if (!offsetInput) {
        return;
    }
    offsetInput.value = '0';
    if (searchTimer) {
        window.clearTimeout(searchTimer);
    }
    searchTimer = window.setTimeout(() => triggerClientsRefresh('filters-changed'), 275);
}

/**
 * Handle page metadata update event
 */
function handlePageMetaUpdate(event) {
    const meta = event.detail || {};
    if (typeof meta.limit === 'number' && limitInput) {
        limitInput.value = String(meta.limit);
    }
    if (typeof meta.offset === 'number' && offsetInput) {
        offsetInput.value = String(meta.offset);
    }
    const hasMore = typeof meta.has_more === 'boolean'
        ? meta.has_more
        : (typeof meta.count === 'number' && typeof meta.limit === 'number' ? meta.count >= meta.limit : true);
    updatePaginationControls({
        limit: Number(limitInput?.value) || paginationMeta.limit,
        offset: Number(offsetInput?.value) || 0,
        has_more: hasMore,
    });
    attachClientSorting();
}

/**
 * Attach sorting handlers to table headers
 */
function attachClientSorting() {
    const table = document.querySelector('.clients-table');
    if (!table) return;
    const tbody = table.querySelector('tbody');
    if (!tbody) return;
    const headers = table.querySelectorAll('th');
    const map = ['client', 'group', 'total', 'blocked', 'nxdomain', 'last'];
    headers.forEach((th, idx) => {
        const key = map[idx];
        if (!key) return;
        th.classList.add('sortable');
        th.addEventListener('click', () => sortClients(tbody, key, table));
    });
}

/**
 * Sort clients table by column
 */
function sortClients(tbody, key, table) {
    const rows = Array.from(tbody.querySelectorAll('tr')).filter((row) => row.dataset && row.dataset.client);
    if (!rows.length) return;
    const currentKey = table.dataset.sortKey;
    const currentDir = table.dataset.sortDir || 'asc';
    const nextDir = currentKey === key && currentDir === 'asc' ? 'desc' : 'asc';
    table.dataset.sortKey = key;
    table.dataset.sortDir = nextDir;

    const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
    rows.sort((a, b) => {
        const va = a.dataset[key] || '';
        const vb = b.dataset[key] || '';
        if (key === 'total' || key === 'blocked' || key === 'nxdomain' || key === 'last') {
            const na = parseFloat(va) || 0;
            const nb = parseFloat(vb) || 0;
            return nextDir === 'asc' ? na - nb : nb - na;
        }
        return nextDir === 'asc' ? collator.compare(va, vb) : collator.compare(vb, va);
    });

    // Clear tbody safely
    while (tbody.firstChild) {
        tbody.removeChild(tbody.firstChild);
    }
    rows.forEach((row) => tbody.appendChild(row));
}
