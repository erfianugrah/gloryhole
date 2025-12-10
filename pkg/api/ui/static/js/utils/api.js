/**
 * API Utility Functions
 * Centralized API communication helpers
 */

/**
 * Fetch JSON from an endpoint with error handling
 * @param {string} url - The API endpoint
 * @param {RequestInit} options - Fetch options
 * @returns {Promise<any>} The parsed JSON response
 */
export async function fetchJSON(url, options = {}) {
    try {
        const response = await fetch(url, options);

        if (!response.ok) {
            let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
            try {
                const errorData = await response.text();
                if (errorData) {
                    errorMessage = errorData;
                }
            } catch (_) {
                // Ignore parse errors for error messages
            }
            throw new Error(errorMessage);
        }

        // Handle 204 No Content
        if (response.status === 204) {
            return {};
        }

        return await response.json();
    } catch (error) {
        console.error('API request failed:', url, error);
        throw error;
    }
}

/**
 * Fetch stats from the API
 * @param {string} period - Time period (e.g., '1h', '24h')
 * @returns {Promise<Object>} Stats data
 */
export async function fetchStats(period = '24h') {
    return fetchJSON(`/api/stats?period=${period}`);
}

/**
 * Fetch time series data
 * @param {string} period - Time period
 * @returns {Promise<Object>} Time series data
 */
export async function fetchTimeSeries(period = '1h') {
    return fetchJSON(`/api/stats/timeseries?period=${period}`);
}

/**
 * Fetch query types distribution
 * @param {string} since - Time range
 * @returns {Promise<Object>} Query types data
 */
export async function fetchQueryTypes(since = '24h') {
    return fetchJSON(`/api/query-types?since=${since}`);
}

/**
 * Fetch top domains
 * @param {number} limit - Number of domains
 * @param {boolean} blocked - Whether to fetch blocked domains
 * @returns {Promise<Array>} Top domains list
 */
export async function fetchTopDomains(limit = 10, blocked = false) {
    return fetchJSON(`/api/top-domains?limit=${limit}&blocked=${blocked}`);
}

/**
 * POST request helper
 * @param {string} url - The API endpoint
 * @param {any} data - Data to send
 * @returns {Promise<any>} The parsed JSON response
 */
export async function postJSON(url, data) {
    return fetchJSON(url, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
    });
}

/**
 * PUT request helper
 * @param {string} url - The API endpoint
 * @param {any} data - Data to send
 * @returns {Promise<any>} The parsed JSON response
 */
export async function putJSON(url, data) {
    return fetchJSON(url, {
        method: 'PUT',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
    });
}

/**
 * DELETE request helper
 * @param {string} url - The API endpoint
 * @returns {Promise<any>} The parsed JSON response
 */
export async function deleteJSON(url) {
    return fetchJSON(url, {
        method: 'DELETE'
    });
}
