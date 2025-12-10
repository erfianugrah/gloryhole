/**
 * Settings Page Module
 * Manages settings page functionality including storage reset,
 * blocklist reload, and feature toggles
 */

// Countdown interval references
let blocklistCountdownInterval = null;
let policiesCountdownInterval = null;

/**
 * Initialize settings page
 */
export function initializeSettings() {
    setupEventListeners();
    updateFeatureStatus();
}

/**
 * Set up event listeners
 */
function setupEventListeners() {
    // Storage reset button
    const resetButton = document.getElementById('storage-nuke-btn');
    if (resetButton) {
        resetButton.addEventListener('click', resetStorage);
    }

    // The reload blocklists button uses inline onclick,
    // but we could refactor it later if needed
}

/**
 * Reset storage with confirmation
 */
async function resetStorage() {
    const input = document.getElementById('storage-nuke-confirm');
    const button = document.getElementById('storage-nuke-btn');
    if (!input || !button) {
        return;
    }

    const value = (input.value || '').trim().toUpperCase();
    if (value !== 'NUKE') {
        alert('Please type "NUKE" to confirm.');
        input.focus();
        return;
    }

    if (!confirm('This will permanently delete all stored query data. Continue?')) {
        return;
    }

    const defaultLabel = button.textContent;
    button.disabled = true;
    button.textContent = 'Deleting...';

    try {
        const response = await fetch('/api/storage/reset', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ confirm: 'NUKE' })
        });

        if (!response.ok) {
            let errorMessage = 'Failed to reset storage';
            try {
                const error = await response.json();
                errorMessage = error.message || errorMessage;
            } catch (_) {
                // ignore JSON parse errors
            }
            throw new Error(errorMessage);
        }

        input.value = '';
        alert('All stored query data has been deleted.');
    } catch (error) {
        alert('Error: ' + error.message);
    } finally {
        button.disabled = false;
        button.textContent = defaultLabel;
    }
}

/**
 * Reload blocklists
 * Note: Called from inline onclick in template
 */
window.reloadBlocklists = async function() {
    const button = event.target;
    button.disabled = true;
    button.textContent = 'Reloading...';

    try {
        const response = await fetch('/api/blocklist/reload', {
            method: 'POST'
        });

        if (response.ok) {
            const result = await response.json();
            alert(`Blocklists reloaded successfully!\n${result.domains} domains loaded.`);
        } else {
            const error = await response.json();
            alert('Error: ' + (error.message || 'Failed to reload blocklists'));
        }
    } catch (error) {
        alert('Error: ' + error.message);
    } finally {
        button.disabled = false;
        button.textContent = 'Reload Now';
    }
};

/**
 * Disable a feature temporarily
 * Note: Called from inline onclick in template
 * @param {string} feature - 'blocklist' or 'policies'
 * @param {number} duration - Duration in seconds (0 for indefinite)
 */
window.disableFeature = async function(feature, duration) {
    const featureName = feature === 'blocklist' ? 'Blocklist' : 'Policies';
    const durationText = duration === 0 ? 'indefinitely' : formatDuration(duration);

    if (!confirm(`Disable ${featureName} ${durationText}?`)) {
        return;
    }

    try {
        const response = await fetch(`/api/features/${feature}/disable`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ duration: duration })
        });

        if (response.ok) {
            await response.json();
            updateFeatureStatus();
        } else {
            const error = await response.json();
            alert('Error: ' + (error.message || `Failed to disable ${featureName}`));
        }
    } catch (error) {
        alert('Error: ' + error.message);
    }
};

/**
 * Enable a feature
 * Note: Called from inline onclick in template
 * @param {string} feature - 'blocklist' or 'policies'
 */
window.enableFeature = async function(feature) {
    const featureName = feature === 'blocklist' ? 'Blocklist' : 'Policies';

    try {
        const response = await fetch(`/api/features/${feature}/enable`, {
            method: 'POST'
        });

        if (response.ok) {
            await response.json();
            updateFeatureStatus();
        } else {
            const error = await response.json();
            alert('Error: ' + (error.message || `Failed to enable ${featureName}`));
        }
    } catch (error) {
        alert('Error: ' + error.message);
    }
};

/**
 * Update feature status from API
 */
async function updateFeatureStatus() {
    try {
        const response = await fetch('/api/features');
        if (!response.ok) {
            throw new Error('Failed to fetch feature status');
        }

        const data = await response.json();
        const blocklistStatus = document.getElementById('blocklist-status');
        const policiesStatus = document.getElementById('policies-status');

        if (blocklistStatus) {
            blocklistStatus.textContent = data.blocklist_enabled ? 'Enabled' : 'Disabled';
            blocklistStatus.className = `status-badge ${data.blocklist_enabled ? 'success' : 'danger'}`;
        }

        if (policiesStatus) {
            policiesStatus.textContent = data.policies_enabled ? 'Enabled' : 'Disabled';
            policiesStatus.className = `status-badge ${data.policies_enabled ? 'success' : 'danger'}`;
        }

        updateCountdown('blocklist', data.blocklist_temp_disabled, data.blocklist_disabled_until);
        updateCountdown('policies', data.policies_temp_disabled, data.policies_disabled_until);
    } catch (error) {
        console.error('Failed to update feature status:', error);
    }
}

/**
 * Format duration in seconds to readable string
 * @param {number} seconds - Duration in seconds
 * @returns {string} Formatted duration
 */
function formatDuration(seconds) {
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
    return `${Math.floor(seconds / 3600)}h`;
}

/**
 * Update countdown timer for temporary feature disable
 * @param {string} feature - 'blocklist' or 'policies'
 * @param {boolean} isDisabled - Whether feature is currently disabled
 * @param {string} until - ISO timestamp when feature re-enables
 */
function updateCountdown(feature, isDisabled, until) {
    const countdownElement = document.getElementById(`${feature}-countdown`);
    const timeElement = document.getElementById(`${feature}-time`);

    if (!countdownElement || !timeElement) {
        return;
    }

    let intervalRef = feature === 'blocklist' ? blocklistCountdownInterval : policiesCountdownInterval;

    if (intervalRef) {
        clearInterval(intervalRef);
    }

    if (!isDisabled || !until) {
        countdownElement.style.display = 'none';
        return;
    }

    countdownElement.style.display = 'block';

    function updateTimer() {
        const remainingSeconds = Math.max(0, Math.floor((new Date(until) - new Date()) / 1000));
        const minutes = Math.floor(remainingSeconds / 60);
        const seconds = remainingSeconds % 60;
        timeElement.textContent = `${minutes}m ${seconds}s`;

        if (remainingSeconds <= 0) {
            clearInterval(intervalRef);
            updateFeatureStatus();
        }
    }

    updateTimer();
    intervalRef = setInterval(updateTimer, 1000);

    if (feature === 'blocklist') {
        blocklistCountdownInterval = intervalRef;
    } else {
        policiesCountdownInterval = intervalRef;
    }
}
