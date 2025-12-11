/**
 * Conditional Forwarding Page Initialization
 * Imports and initializes conditional forwarding page functionality
 */

import { initializeConditionalForwarding } from './modules/conditionalforwarding.js';

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    initializeConditionalForwarding();
});
