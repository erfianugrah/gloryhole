/**
 * Conditional Forwarding Page Module
 * Manages conditional forwarding rules including adding/removing rules
 * and modal management
 */

/**
 * Initialize conditional forwarding page
 */
export function initializeConditionalForwarding() {
    setupEventListeners();
}

/**
 * Set up event listeners
 */
function setupEventListeners() {
    // Close modals on escape key
    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') {
            closeAddModal();
        }
    });

    // Close modal when clicking outside
    const addModal = document.getElementById('add-modal');
    if (addModal) {
        addModal.addEventListener('click', function(e) {
            if (e.target === addModal) {
                closeAddModal();
            }
        });
    }
}

/**
 * Show add rule modal
 */
window.showAddModal = function() {
    const modal = document.getElementById('add-modal');
    const nameInput = document.getElementById('name-input');

    if (modal) {
        modal.style.display = 'flex';
        if (nameInput) {
            nameInput.focus();
        }
    }
};

/**
 * Close add rule modal
 */
window.closeAddModal = function() {
    const modal = document.getElementById('add-modal');
    const form = document.getElementById('add-form');

    if (modal) {
        modal.style.display = 'none';
    }

    if (form) {
        form.reset();
    }
};

/**
 * Submit add rule form
 * @param {Event} event - Form submit event
 */
window.submitAdd = async function(event) {
    event.preventDefault();

    const form = event.target;
    const submitButton = form.querySelector('button[type="submit"]');

    if (!submitButton) return;

    // Gather form data
    const name = document.getElementById('name-input')?.value.trim();
    const priority = parseInt(document.getElementById('priority-input')?.value) || 50;
    const failover = document.getElementById('failover-input')?.checked || false;

    // Matching conditions
    const domainsText = document.getElementById('domains-input')?.value.trim();
    const clientCIDRsText = document.getElementById('client-cidrs-input')?.value.trim();
    const queryTypesText = document.getElementById('query-types-input')?.value.trim();

    // Upstream configuration
    const upstreamsText = document.getElementById('upstreams-input')?.value.trim();
    const timeout = document.getElementById('timeout-input')?.value.trim();
    const maxRetries = parseInt(document.getElementById('max-retries-input')?.value) || 2;

    // Validation
    if (!name) {
        alert('Please enter a rule name');
        document.getElementById('name-input')?.focus();
        return;
    }

    if (!upstreamsText) {
        alert('Please enter at least one upstream server');
        document.getElementById('upstreams-input')?.focus();
        return;
    }

    if (!domainsText && !clientCIDRsText && !queryTypesText) {
        alert('Please specify at least one matching condition (domains, client networks, or query types)');
        return;
    }

    // Build request payload
    const payload = {
        name,
        priority,
        failover,
        max_retries: maxRetries
    };

    // Parse domains (one per line)
    if (domainsText) {
        payload.domains = domainsText.split('\n').map(d => d.trim()).filter(d => d);
    }

    // Parse client CIDRs (one per line)
    if (clientCIDRsText) {
        payload.client_cidrs = clientCIDRsText.split('\n').map(c => c.trim()).filter(c => c);
    }

    // Parse query types (comma-separated)
    if (queryTypesText) {
        payload.query_types = queryTypesText.split(',').map(q => q.trim().toUpperCase()).filter(q => q);
    }

    // Parse upstreams (one per line)
    payload.upstreams = upstreamsText.split('\n').map(u => u.trim()).filter(u => u);

    // Add timeout if specified
    if (timeout) {
        payload.timeout = timeout;
    }

    const defaultLabel = submitButton.textContent;
    submitButton.disabled = true;
    submitButton.textContent = 'Adding...';

    try {
        const response = await fetch('/api/conditionalforwarding', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(payload)
        });

        if (!response.ok) {
            let errorMessage = 'Failed to add rule';
            try {
                const error = await response.json();
                errorMessage = error.message || errorMessage;
            } catch (_) {
                // ignore JSON parse errors
            }
            throw new Error(errorMessage);
        }

        // Success - close modal and trigger HTMX update
        closeAddModal();
        document.body.dispatchEvent(new CustomEvent('conditionalforwarding-updated'));

        // Show success message
        showNotification('Forwarding rule added successfully', 'success');
    } catch (error) {
        alert('Error: ' + error.message);
    } finally {
        submitButton.disabled = false;
        submitButton.textContent = defaultLabel;
    }
};

/**
 * Remove a forwarding rule
 * @param {string} ruleId - Rule ID to remove
 */
window.removeRule = async function(ruleId) {
    if (!confirm('Remove this forwarding rule?')) {
        return;
    }

    try {
        const response = await fetch(`/api/conditionalforwarding/${encodeURIComponent(ruleId)}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            let errorMessage = 'Failed to remove rule';
            try {
                const error = await response.json();
                errorMessage = error.message || errorMessage;
            } catch (_) {
                // ignore JSON parse errors
            }
            throw new Error(errorMessage);
        }

        // Success - trigger HTMX update
        document.body.dispatchEvent(new CustomEvent('conditionalforwarding-updated'));

        // Show success message
        showNotification('Forwarding rule removed successfully', 'success');
    } catch (error) {
        alert('Error: ' + error.message);
    }
};

/**
 * Show notification message
 * @param {string} message - Message to display
 * @param {string} type - Notification type (success, error, info)
 */
function showNotification(message, type = 'info') {
    // For now, use console log
    // TODO: Implement a proper notification toast system
    console.log(`[${type.toUpperCase()}] ${message}`);
}
