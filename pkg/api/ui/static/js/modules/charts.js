/**
 * Dashboard Charts Module
 * Manages all Chart.js visualizations on the dashboard
 */

import { fetchTimeSeries, fetchQueryTypes, fetchTopDomains } from '../utils/api.js';
import { renderLegend, datasetLegendItems, createLegendItems } from '../utils/chart-legend.js';

// Chart instances
let queryChart = null;
let cacheHitChart = null;
let queryTypeChart = null;
let topAllowedChart = null;
let topBlockedChart = null;

// Query type filtering state
let queryTypeLabels = [];
let queryTypeCounts = [];
let queryTypeColors = [];
const queryTypeHidden = new Set();

/**
 * Initialize all dashboard charts
 */
export function initializeCharts() {
    initializeTimeSeriesCharts();
    initializeTypeChart();
    initializeDomainCharts();
}

/**
 * Start auto-refresh for all charts
 */
export function startAutoRefresh() {
    // Initial data load
    refreshTimeSeriesCharts();
    updateQueryTypeChart();
    updateTopDomainsCharts();

    // Set up intervals
    setInterval(refreshTimeSeriesCharts, 30000); // 30 seconds
    setInterval(updateQueryTypeChart, 60000); // 60 seconds
    setInterval(updateTopDomainsCharts, 45000); // 45 seconds
}

/**
 * Initialize time series charts (Query and Cache Hit Rate)
 */
function initializeTimeSeriesCharts() {
    const queryCtx = document.getElementById('queryChart');
    if (queryCtx) {
        queryChart = new Chart(queryCtx, {
            type: 'line',
            data: {
                labels: [],
                datasets: [
                    {
                        label: 'Total Queries',
                        data: [],
                        borderColor: '#3b82f6',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        tension: 0.4,
                        fill: true
                    },
                    {
                        label: 'Blocked',
                        data: [],
                        borderColor: '#ef4444',
                        backgroundColor: 'rgba(239, 68, 68, 0.1)',
                        tension: 0.4,
                        fill: true
                    },
                    {
                        label: 'Cached',
                        data: [],
                        borderColor: '#10b981',
                        backgroundColor: 'rgba(16, 185, 129, 0.1)',
                        tension: 0.4,
                        fill: true
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: {
                    mode: 'index',
                    intersect: false
                },
                plugins: {
                    legend: {
                        display: false
                    }
                },
                scales: {
                    x: {
                        display: true,
                        title: { display: true, text: 'Time' }
                    },
                    y: {
                        display: true,
                        title: { display: true, text: 'Queries' },
                        beginAtZero: true
                    }
                }
            }
        });
        renderLegend('queryChart-legend', datasetLegendItems(queryChart.data.datasets));
    }

    const cacheCtx = document.getElementById('cacheHitChart');
    if (cacheCtx) {
        cacheHitChart = new Chart(cacheCtx, {
            type: 'line',
            data: {
                labels: [],
                datasets: [{
                    label: 'Hit Rate (%)',
                    data: [],
                    borderColor: '#6366f1',
                    backgroundColor: 'rgba(99, 102, 241, 0.15)',
                    tension: 0.3,
                    fill: true
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false }
                },
                scales: {
                    x: {
                        display: true,
                        title: { display: true, text: 'Time' }
                    },
                    y: {
                        display: true,
                        title: { display: true, text: 'Percentage' },
                        beginAtZero: true,
                        suggestedMax: 100
                    }
                }
            }
        });
        renderLegend('cacheHitChart-legend', datasetLegendItems(cacheHitChart.data.datasets));
    }
}

/**
 * Refresh time series charts with latest data
 */
async function refreshTimeSeriesCharts() {
    if (!queryChart && !cacheHitChart) return;

    try {
        const response = await fetch('/api/stats/timeseries?period=hour&points=24', {
            credentials: 'same-origin'
        });
        if (!response.ok) {
            const errorText = await response.text().catch(() => response.statusText);
            throw new Error(`Failed to load time-series data: ${response.status} ${errorText}`);
        }

        const payload = await response.json();
        const labels = [];
        const total = [];
        const blocked = [];
        const cached = [];
        const hitRates = [];

        (payload.data || []).forEach(point => {
            const ts = new Date(point.timestamp);
            labels.push(ts.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            total.push(point.total_queries);
            blocked.push(point.blocked_queries);
            cached.push(point.cached_queries);
            const rate = point.total_queries > 0 ? (point.cached_queries / point.total_queries) * 100 : 0;
            hitRates.push(Number.isFinite(rate) ? Number(rate.toFixed(2)) : 0);
        });

        if (queryChart) {
            queryChart.data.labels = labels;
            queryChart.data.datasets[0].data = total;
            queryChart.data.datasets[1].data = blocked;
            queryChart.data.datasets[2].data = cached;
            queryChart.update('none');
            renderLegend('queryChart-legend', datasetLegendItems(queryChart.data.datasets));
        }

        if (cacheHitChart) {
            cacheHitChart.data.labels = labels;
            cacheHitChart.data.datasets[0].data = hitRates;
            cacheHitChart.update('none');
            renderLegend('cacheHitChart-legend', datasetLegendItems(cacheHitChart.data.datasets));
        }
    } catch (error) {
        console.error('Failed to update time-series charts:', error);
    }
}

/**
 * Initialize query type doughnut chart
 */
function initializeTypeChart() {
    const ctx = document.getElementById('queryTypeChart');
    if (!ctx) return;

    queryTypeChart = new Chart(ctx, {
        type: 'doughnut',
        data: {
            labels: [],
            datasets: [{
                data: [],
                backgroundColor: [
                    '#3b82f6',
                    '#10b981',
                    '#f59e0b',
                    '#ef4444',
                    '#6366f1',
                    '#14b8a6'
                ],
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    display: false
                }
            }
        }
    });
    renderLegend('queryTypeChart-legend', []);
}

/**
 * Update query type chart with latest data
 */
async function updateQueryTypeChart() {
    if (!queryTypeChart) return;

    try {
        const params = new URLSearchParams({ limit: 6 });
        const queryTypeRangeSelect = document.getElementById('query-type-range');
        const selectedRange = queryTypeRangeSelect?.value;
        if (selectedRange) {
            params.set('since', selectedRange);
        }

        const response = await fetch(`/api/stats/query-types?${params.toString()}`, {
            credentials: 'same-origin'
        });
        if (!response.ok) {
            const errorText = await response.text().catch(() => response.statusText);
            throw new Error(`Failed to load query-type data: ${response.status} ${errorText}`);
        }

        const payload = await response.json();
        const labels = [];
        const counts = [];

        (payload.types || []).forEach(item => {
            labels.push(item.query_type);
            counts.push(item.total);
        });

        const colors = Array.isArray(queryTypeChart.data.datasets[0].backgroundColor)
            ? queryTypeChart.data.datasets[0].backgroundColor
            : [];
        queryTypeLabels = labels.slice();
        queryTypeCounts = counts.slice();
        queryTypeColors = colors.slice();
        pruneQueryTypeHidden();
        applyQueryTypeFilters();
    } catch (error) {
        console.error('Failed to update query-type chart:', error);
    }
}

/**
 * Remove hidden query types that no longer exist in the dataset
 */
function pruneQueryTypeHidden() {
    if (!queryTypeLabels.length) {
        queryTypeHidden.clear();
        return;
    }
    const valid = new Set(queryTypeLabels);
    Array.from(queryTypeHidden).forEach((label) => {
        if (!valid.has(label)) {
            queryTypeHidden.delete(label);
        }
    });
    if (queryTypeHidden.size >= queryTypeLabels.length) {
        queryTypeHidden.clear();
    }
}

/**
 * Apply query type filters (show/hide slices)
 */
function applyQueryTypeFilters() {
    if (!queryTypeChart) return;
    const dataset = queryTypeChart.data.datasets[0];
    dataset.data = queryTypeLabels.map((label, idx) => {
        const value = queryTypeCounts[idx] || 0;
        return queryTypeHidden.has(label) ? 0 : value;
    });
    queryTypeChart.data.labels = queryTypeLabels.slice();
    queryTypeChart.update('none');
    const legendItems = queryTypeLabels.map((label, idx) => ({
        label,
        color: queryTypeColors[idx] || '#3b82f6',
    }));
    renderLegend('queryTypeChart-legend', legendItems, {
        isActive: (item) => !queryTypeHidden.has(item.label),
        onClick: (item) => {
            if (queryTypeHidden.has(item.label)) {
                queryTypeHidden.delete(item.label);
            } else if (queryTypeHidden.size < queryTypeLabels.length - 1) {
                queryTypeHidden.add(item.label);
            } else {
                return;
            }
            applyQueryTypeFilters();
        }
    });
}

/**
 * Initialize top domains bar charts
 */
function initializeDomainCharts() {
    const allowedCtx = document.getElementById('topAllowedChart');
    if (allowedCtx) {
        topAllowedChart = new Chart(allowedCtx, createDomainBarConfig('#00ff41'));
        renderLegend('topAllowedChart-legend', [{ label: 'Queries', color: '#00ff41' }]);
    }

    const blockedCtx = document.getElementById('topBlockedChart');
    if (blockedCtx) {
        topBlockedChart = new Chart(blockedCtx, createDomainBarConfig('#ff006e'));
        renderLegend('topBlockedChart-legend', [{ label: 'Blocked', color: '#ff006e' }]);
    }
}

/**
 * Create config for domain bar chart
 * @param {string} color - Bar color
 * @returns {Object} Chart.js config
 */
function createDomainBarConfig(color) {
    return {
        type: 'bar',
        data: {
            labels: [],
            datasets: [{
                data: [],
                backgroundColor: color
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        // Show full domain in tooltip
                        title: function(context) {
                            return context[0].label;
                        }
                    }
                }
            },
            scales: {
                x: {
                    ticks: {
                        autoSkip: false,
                        maxRotation: 45,
                        minRotation: 45,
                        // Truncate long domain names
                        callback: function(value, index, ticks) {
                            const label = this.getLabelForValue(value);
                            const maxLength = 20;
                            if (label.length > maxLength) {
                                return label.substring(0, maxLength) + '...';
                            }
                            return label;
                        }
                    }
                },
                y: {
                    beginAtZero: true,
                    suggestedMax: 10
                }
            }
        }
    };
}

/**
 * Update both top domains charts
 */
async function updateTopDomainsCharts() {
    const allowedRange = document.getElementById('top-allowed-range')?.value || '';
    const blockedRange = document.getElementById('top-blocked-range')?.value || '';

    await Promise.all([
        updateTopDomainsChart('/api/top-domains?limit=6&blocked=false', topAllowedChart, allowedRange),
        updateTopDomainsChart('/api/top-domains?limit=6&blocked=true', topBlockedChart, blockedRange),
    ]);
}

/**
 * Update a single top domains chart
 * @param {string} baseUrl - Base API endpoint
 * @param {Chart} chart - Chart.js instance
 * @param {string} range - Time range (e.g., '24h', '7d')
 */
async function updateTopDomainsChart(baseUrl, chart, range = '') {
    if (!chart) return;

    try {
        const url = range ? `${baseUrl}&since=${range}` : baseUrl;
        const response = await fetch(url, {
            credentials: 'same-origin'
        });
        if (!response.ok) {
            const errorText = await response.text().catch(() => response.statusText);
            throw new Error(`Failed to load top domains data: ${response.status} ${errorText}`);
        }

        const payload = await response.json();
        const labels = [];
        const values = [];

        (payload.domains || []).forEach(domain => {
            labels.push(domain.domain);
            values.push(domain.queries);
        });

        chart.data.labels = labels;
        chart.data.datasets[0].data = values;
        const legendId = chart === topBlockedChart ? 'topBlockedChart-legend' : 'topAllowedChart-legend';
        const color = chart?.data?.datasets?.[0]?.backgroundColor || '#3b82f6';
        renderLegend(legendId, [{ label: 'Queries', color }]);
        chart.update('none');
    } catch (error) {
        console.error('Failed to update top domains chart:', error);
    }
}

/**
 * Set up top domains range selector event listeners
 */
export function setupTopDomainsRangeSelectors() {
    const allowedRangeSelect = document.getElementById('top-allowed-range');
    const blockedRangeSelect = document.getElementById('top-blocked-range');

    if (allowedRangeSelect) {
        allowedRangeSelect.addEventListener('change', () => {
            // Mark as custom when manually changed
            allowedRangeSelect.dataset.usesGlobal = 'false';
            updateTopDomainsCharts();
        });
    }

    if (blockedRangeSelect) {
        blockedRangeSelect.addEventListener('change', () => {
            // Mark as custom when manually changed
            blockedRangeSelect.dataset.usesGlobal = 'false';
            updateTopDomainsCharts();
        });
    }
}

/**
 * Set up global range selector
 * When changed, updates all local selectors that are using the global default
 */
export function setupGlobalRangeSelector() {
    const globalRangeSelect = document.getElementById('global-range');
    if (!globalRangeSelect) return;

    globalRangeSelect.addEventListener('change', () => {
        const globalValue = globalRangeSelect.value;

        // Update all local selectors that are using global default
        const localSelectors = document.querySelectorAll('.local-range-select');
        localSelectors.forEach(selector => {
            if (selector.dataset.usesGlobal === 'true') {
                selector.value = globalValue;

                // For HTMX-enabled selectors, trigger the HTMX request
                if (selector.hasAttribute('hx-get')) {
                    // Use htmx.trigger to trigger the HTMX request
                    if (typeof htmx !== 'undefined') {
                        htmx.trigger(selector, 'change');
                    }
                } else {
                    // For regular selectors (charts), trigger change event
                    const changeEvent = new Event('change', { bubbles: true });
                    selector.dispatchEvent(changeEvent);
                }
            }
        });
    });

    // Set up listeners on local selectors to mark them as custom when manually changed
    const localSelectors = document.querySelectorAll('.local-range-select');
    localSelectors.forEach(selector => {
        // Skip if it already has a listener (for chart selectors)
        const isChartSelector = ['query-type-range', 'top-allowed-range', 'top-blocked-range'].includes(selector.id);
        if (!isChartSelector) {
            selector.addEventListener('change', () => {
                // Mark as custom when manually changed
                selector.dataset.usesGlobal = 'false';
            });
        }
    });
}

/**
 * Update query type selector to mark as custom when changed
 */
export function setupQueryTypeRangeSelector() {
    const queryTypeRangeSelect = document.getElementById('query-type-range');
    if (queryTypeRangeSelect) {
        queryTypeRangeSelect.addEventListener('change', () => {
            // Mark as custom when manually changed
            queryTypeRangeSelect.dataset.usesGlobal = 'false';
            updateQueryTypeChart();
        });
    }
}
