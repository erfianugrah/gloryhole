/**
 * Whitelist Page Initialization
 * Imports and initializes whitelist page functionality
 */

import { initializeWhitelist } from './modules/whitelist.js';

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    initializeWhitelist();
});
