/**
 * Chart Legend Utility
 * Renders custom legends for Chart.js charts
 */

/**
 * Render a custom legend for a chart
 * @param {string} containerId - The ID of the legend container element
 * @param {Array} items - Array of legend items {label, color}
 * @param {Object} options - Options for legend rendering
 * @param {Function} options.isActive - Function to check if item is active
 * @param {Function} options.onClick - Click handler for legend items
 */
export function renderLegend(containerId, items, options = {}) {
    const container = document.getElementById(containerId);
    if (!container) {
        console.warn(`Legend container not found: ${containerId}`);
        return;
    }

    // Clear container safely by removing all children
    while (container.firstChild) {
        container.removeChild(container.firstChild);
    }

    if (!Array.isArray(items) || items.length === 0) {
        return;
    }

    items.forEach((item, index) => {
        if (!item || !item.label) return;

        const chip = document.createElement('div');
        chip.className = 'legend-chip';

        // Set custom color
        if (item.color) {
            chip.style.setProperty('--legend-color', item.color);
        }

        chip.textContent = item.label;

        // Mark as inactive if needed
        if (typeof options.isActive === 'function' && !options.isActive(item, index)) {
            chip.classList.add('legend-chip-inactive');
        }

        // Make clickable if onClick provided
        if (typeof options.onClick === 'function') {
            chip.classList.add('legend-chip-button');
            chip.setAttribute('role', 'button');
            chip.tabIndex = 0;

            // Mouse click
            chip.addEventListener('click', (event) => {
                event.preventDefault();
                options.onClick(item, index, event);
            });

            // Keyboard support (Enter or Space)
            chip.addEventListener('keydown', (event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    options.onClick(item, index, event);
                }
            });
        }

        container.appendChild(chip);
    });
}

/**
 * Convert Chart.js datasets to legend items
 * @param {Array} datasets - Array of Chart.js datasets
 * @returns {Array} Array of legend items {label, color}
 */
export function datasetLegendItems(datasets) {
    if (!Array.isArray(datasets)) {
        return [];
    }

    return datasets.map((ds) => ({
        label: ds.label,
        color: Array.isArray(ds.borderColor)
            ? ds.borderColor[0]
            : ds.borderColor || ds.backgroundColor || '#3b82f6',
    }));
}

/**
 * Create legend items from labels and colors arrays
 * @param {Array} labels - Array of labels
 * @param {Array} colors - Array of colors
 * @returns {Array} Array of legend items {label, color}
 */
export function createLegendItems(labels, colors) {
    if (!Array.isArray(labels)) {
        return [];
    }

    return labels.map((label, index) => ({
        label,
        color: colors && colors[index] ? colors[index] : '#3b82f6'
    }));
}
