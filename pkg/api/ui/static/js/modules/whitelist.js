/**
 * Whitelist Page Module
 * Manages whitelist page functionality including adding/removing domains,
 * bulk import, and modal management
 */

/**
 * Initialize whitelist page
 */
export function initializeWhitelist() {
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
            closeImportModal();
        }
    });

    // Close modals when clicking outside
    const addModal = document.getElementById('add-modal');
    const importModal = document.getElementById('import-modal');

    if (addModal) {
        addModal.addEventListener('click', function(e) {
            if (e.target === addModal) {
                closeAddModal();
            }
        });
    }

    if (importModal) {
        importModal.addEventListener('click', function(e) {
            if (e.target === importModal) {
                closeImportModal();
            }
        });
    }
}

/**
 * Show add domain modal
 */
window.showAddModal = function() {
    const modal = document.getElementById('add-modal');
    const input = document.getElementById('domain-input');

    if (modal) {
        modal.style.display = 'flex';
        if (input) {
            input.focus();
        }
    }
};

/**
 * Close add domain modal
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
 * Show import domains modal
 */
window.showImportModal = function() {
    const modal = document.getElementById('import-modal');
    const textarea = document.getElementById('domains-input');

    if (modal) {
        modal.style.display = 'flex';
        if (textarea) {
            textarea.focus();
        }
    }
};

/**
 * Close import domains modal
 */
window.closeImportModal = function() {
    const modal = document.getElementById('import-modal');
    const form = document.getElementById('import-form');

    if (modal) {
        modal.style.display = 'none';
    }

    if (form) {
        form.reset();
    }
};

/**
 * Submit add domain form
 * @param {Event} event - Form submit event
 */
window.submitAdd = async function(event) {
    event.preventDefault();

    const form = event.target;
    const input = document.getElementById('domain-input');
    const submitButton = form.querySelector('button[type="submit"]');

    if (!input || !submitButton) {
        return;
    }

    const domain = input.value.trim();
    if (!domain) {
        alert('Please enter a domain');
        input.focus();
        return;
    }

    const defaultLabel = submitButton.textContent;
    submitButton.disabled = true;
    submitButton.textContent = 'Adding...';

    try {
        const response = await fetch('/api/whitelist', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                domains: [domain]
            })
        });

        if (!response.ok) {
            let errorMessage = 'Failed to add domain';
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
        document.body.dispatchEvent(new CustomEvent('whitelist-updated'));

        // Show success message
        showNotification('Domain added successfully', 'success');
    } catch (error) {
        alert('Error: ' + error.message);
    } finally {
        submitButton.disabled = false;
        submitButton.textContent = defaultLabel;
    }
};

/**
 * Submit import domains form
 * @param {Event} event - Form submit event
 */
window.submitImport = async function(event) {
    event.preventDefault();

    const form = event.target;
    const textarea = document.getElementById('domains-input');
    const submitButton = form.querySelector('button[type="submit"]');

    if (!textarea || !submitButton) {
        return;
    }

    const text = textarea.value.trim();
    if (!text) {
        alert('Please enter at least one domain');
        textarea.focus();
        return;
    }

    const defaultLabel = submitButton.textContent;
    submitButton.disabled = true;
    submitButton.textContent = 'Importing...';

    try {
        const response = await fetch('/api/whitelist/bulk', {
            method: 'POST',
            headers: {
                'Content-Type': 'text/plain',
            },
            body: text
        });

        if (!response.ok) {
            let errorMessage = 'Failed to import domains';
            try {
                const error = await response.json();
                errorMessage = error.message || errorMessage;
            } catch (_) {
                // ignore JSON parse errors
            }
            throw new Error(errorMessage);
        }

        // Success - close modal and trigger HTMX update
        closeImportModal();
        document.body.dispatchEvent(new CustomEvent('whitelist-updated'));

        // Show success message
        showNotification('Domains imported successfully', 'success');
    } catch (error) {
        alert('Error: ' + error.message);
    } finally {
        submitButton.disabled = false;
        submitButton.textContent = defaultLabel;
    }
};

/**
 * Remove a domain from whitelist
 * @param {string} domain - Domain to remove
 */
window.removeDomain = async function(domain) {
    if (!confirm(`Remove "${domain}" from whitelist?`)) {
        return;
    }

    try {
        const response = await fetch(`/api/whitelist/${encodeURIComponent(domain)}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            let errorMessage = 'Failed to remove domain';
            try {
                const error = await response.json();
                errorMessage = error.message || errorMessage;
            } catch (_) {
                // ignore JSON parse errors
            }
            throw new Error(errorMessage);
        }

        // Success - trigger HTMX update
        document.body.dispatchEvent(new CustomEvent('whitelist-updated'));

        // Show success message
        showNotification('Domain removed successfully', 'success');
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
    // For now, use browser alert
    // TODO: Implement a proper notification toast system
    console.log(`[${type.toUpperCase()}] ${message}`);
}
