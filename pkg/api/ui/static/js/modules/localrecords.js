/**
 * Local Records Page Module
 * Manages local DNS records functionality including adding/removing records,
 * type-specific form handling, and modal management
 */

/**
 * Initialize local records page
 */
export function initializeLocalRecords() {
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
 * Show add record modal
 */
window.showAddModal = function() {
    const modal = document.getElementById('add-modal');
    const domainInput = document.getElementById('domain-input');

    if (modal) {
        modal.style.display = 'flex';
        if (domainInput) {
            domainInput.focus();
        }
    }
};

/**
 * Close add record modal
 */
window.closeAddModal = function() {
    const modal = document.getElementById('add-modal');
    const form = document.getElementById('add-form');

    if (modal) {
        modal.style.display = 'none';
    }

    if (form) {
        form.reset();
        // Hide all conditional fields
        hideAllConditionalFields();
    }
};

/**
 * Update form fields based on selected record type
 */
window.updateFormFields = function() {
    const typeSelect = document.getElementById('type-input');
    if (!typeSelect) return;

    const selectedType = typeSelect.value.toUpperCase();

    // Hide all conditional fields first
    hideAllConditionalFields();

    // Show relevant fields based on type
    switch (selectedType) {
        case 'A':
        case 'AAAA':
            showField('fields-a-aaaa');
            break;
        case 'CNAME':
        case 'PTR':
            showField('fields-cname-ptr');
            break;
        case 'MX':
            showField('fields-mx');
            break;
        case 'TXT':
            showField('fields-txt');
            break;
        case 'SRV':
            showField('fields-srv');
            break;
    }
};

/**
 * Hide all conditional field groups
 */
function hideAllConditionalFields() {
    const fields = [
        'fields-a-aaaa',
        'fields-cname-ptr',
        'fields-mx',
        'fields-txt',
        'fields-srv'
    ];

    fields.forEach(fieldId => {
        const field = document.getElementById(fieldId);
        if (field) {
            field.style.display = 'none';
        }
    });
}

/**
 * Show a specific field group
 * @param {string} fieldId - ID of field group to show
 */
function showField(fieldId) {
    const field = document.getElementById(fieldId);
    if (field) {
        field.style.display = 'block';
    }
}

/**
 * Submit add record form
 * @param {Event} event - Form submit event
 */
window.submitAdd = async function(event) {
    event.preventDefault();

    const form = event.target;
    const submitButton = form.querySelector('button[type="submit"]');

    if (!submitButton) return;

    // Gather form data
    const domain = document.getElementById('domain-input')?.value.trim();
    const type = document.getElementById('type-input')?.value.toUpperCase();
    const ttl = parseInt(document.getElementById('ttl-input')?.value) || 300;
    const wildcard = document.getElementById('wildcard-input')?.checked || false;

    // Validation
    if (!domain) {
        alert('Please enter a domain');
        document.getElementById('domain-input')?.focus();
        return;
    }

    if (!type) {
        alert('Please select a record type');
        document.getElementById('type-input')?.focus();
        return;
    }

    // Build request payload based on record type
    const payload = {
        domain,
        type,
        ttl,
        wildcard
    };

    try {
        switch (type) {
            case 'A':
            case 'AAAA':
                const ipsText = document.getElementById('ips-input')?.value.trim();
                if (!ipsText) {
                    alert(`Please enter at least one IP address for ${type} record`);
                    return;
                }
                payload.ips = ipsText.split('\n').map(ip => ip.trim()).filter(ip => ip);
                break;

            case 'CNAME':
            case 'PTR':
                const target = document.getElementById('target-input')?.value.trim();
                if (!target) {
                    alert(`Please enter a target for ${type} record`);
                    return;
                }
                payload.target = target;
                break;

            case 'MX':
                const mxTarget = document.getElementById('mx-target-input')?.value.trim();
                const priority = parseInt(document.getElementById('priority-input')?.value);
                if (!mxTarget) {
                    alert('Please enter a mail server for MX record');
                    return;
                }
                payload.target = mxTarget;
                if (!isNaN(priority)) {
                    payload.priority = priority;
                }
                break;

            case 'TXT':
                const txtText = document.getElementById('txt-input')?.value.trim();
                if (!txtText) {
                    alert('Please enter at least one TXT record');
                    return;
                }
                payload.txt_records = txtText.split('\n').map(txt => txt.trim()).filter(txt => txt);
                break;

            case 'SRV':
                const srvTarget = document.getElementById('srv-target-input')?.value.trim();
                const srvPriority = parseInt(document.getElementById('srv-priority-input')?.value);
                const weight = parseInt(document.getElementById('weight-input')?.value);
                const port = parseInt(document.getElementById('port-input')?.value);

                if (!srvTarget) {
                    alert('Please enter a target for SRV record');
                    return;
                }
                if (isNaN(srvPriority) || isNaN(weight) || isNaN(port)) {
                    alert('Please enter priority, weight, and port for SRV record');
                    return;
                }

                payload.target = srvTarget;
                payload.priority = srvPriority;
                payload.weight = weight;
                payload.port = port;
                break;

            default:
                alert(`Unsupported record type: ${type}`);
                return;
        }
    } catch (error) {
        alert('Error building request: ' + error.message);
        return;
    }

    const defaultLabel = submitButton.textContent;
    submitButton.disabled = true;
    submitButton.textContent = 'Adding...';

    try {
        const response = await fetch('/api/localrecords', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(payload)
        });

        if (!response.ok) {
            let errorMessage = 'Failed to add record';
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
        document.body.dispatchEvent(new CustomEvent('localrecords-updated'));

        // Show success message
        showNotification('Local record added successfully', 'success');
    } catch (error) {
        alert('Error: ' + error.message);
    } finally {
        submitButton.disabled = false;
        submitButton.textContent = defaultLabel;
    }
};

/**
 * Remove a local record
 * @param {string} recordId - Record ID to remove
 */
window.removeRecord = async function(recordId) {
    if (!confirm('Remove this local DNS record?')) {
        return;
    }

    try {
        const response = await fetch(`/api/localrecords/${encodeURIComponent(recordId)}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            let errorMessage = 'Failed to remove record';
            try {
                const error = await response.json();
                errorMessage = error.message || errorMessage;
            } catch (_) {
                // ignore JSON parse errors
            }
            throw new Error(errorMessage);
        }

        // Success - trigger HTMX update
        document.body.dispatchEvent(new CustomEvent('localrecords-updated'));

        // Show success message
        showNotification('Local record removed successfully', 'success');
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
