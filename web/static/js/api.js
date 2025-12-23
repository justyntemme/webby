/**
 * Webby Library - API Module
 * Fetch wrappers and API utilities
 */

import { API_BASE } from './config.js';
import { getAuthHeaders } from './auth.js';

/**
 * Make an authenticated GET request
 * @param {string} endpoint - API endpoint (without base)
 * @returns {Promise<Response>}
 */
export async function apiGet(endpoint) {
    return fetch(`${API_BASE}${endpoint}`, {
        headers: getAuthHeaders()
    });
}

/**
 * Make an authenticated POST request with JSON body
 * @param {string} endpoint - API endpoint
 * @param {Object} data - Request body
 * @returns {Promise<Response>}
 */
export async function apiPost(endpoint, data) {
    return fetch(`${API_BASE}${endpoint}`, {
        method: 'POST',
        headers: {
            ...getAuthHeaders(),
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
    });
}

/**
 * Make an authenticated PUT request with JSON body
 * @param {string} endpoint - API endpoint
 * @param {Object} data - Request body
 * @returns {Promise<Response>}
 */
export async function apiPut(endpoint, data) {
    return fetch(`${API_BASE}${endpoint}`, {
        method: 'PUT',
        headers: {
            ...getAuthHeaders(),
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
    });
}

/**
 * Make an authenticated DELETE request
 * @param {string} endpoint - API endpoint
 * @returns {Promise<Response>}
 */
export async function apiDelete(endpoint) {
    return fetch(`${API_BASE}${endpoint}`, {
        method: 'DELETE',
        headers: getAuthHeaders()
    });
}

/**
 * Upload a file with authentication
 * @param {string} endpoint - API endpoint
 * @param {File} file - File to upload
 * @param {Function} onProgress - Progress callback (0-100)
 * @returns {Promise<Object>} Response data
 */
export function uploadFile(endpoint, file, onProgress) {
    return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        const formData = new FormData();
        formData.append('file', file);

        xhr.upload.addEventListener('progress', (e) => {
            if (e.lengthComputable && onProgress) {
                const percent = Math.round((e.loaded / e.total) * 100);
                onProgress(percent);
            }
        });

        xhr.addEventListener('load', () => {
            if (xhr.status >= 200 && xhr.status < 300) {
                try {
                    resolve(JSON.parse(xhr.responseText));
                } catch {
                    resolve({ success: true });
                }
            } else {
                try {
                    reject(JSON.parse(xhr.responseText));
                } catch {
                    reject({ error: `Upload failed with status ${xhr.status}` });
                }
            }
        });

        xhr.addEventListener('error', () => {
            reject({ error: 'Network error during upload' });
        });

        xhr.open('POST', `${API_BASE}${endpoint}`);

        // Add auth header
        const token = localStorage.getItem('webby-token');
        if (token) {
            xhr.setRequestHeader('Authorization', `Bearer ${token}`);
        }

        xhr.send(formData);
    });
}

/**
 * Fetch and parse JSON response
 * @param {string} endpoint - API endpoint
 * @returns {Promise<Object>}
 */
export async function fetchJson(endpoint) {
    const res = await apiGet(endpoint);
    if (!res.ok) {
        throw new Error(`API error: ${res.status}`);
    }
    return res.json();
}
