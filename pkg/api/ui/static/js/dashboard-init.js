/**
 * Dashboard Initialization
 * Imports and initializes dashboard charts
 */

import { initializeCharts, startAutoRefresh, setupQueryTypeRangeSelector } from './modules/charts.js';

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    initializeCharts();
    setupQueryTypeRangeSelector();
    startAutoRefresh();
});
