/**
 * Webby Library - Authentication Module
 * Handles user authentication, tokens, and session management
 */

import { API_BASE } from './config.js';
import { setCurrentUser, getCurrentUser } from './state.js';

const TOKEN_KEY = 'webby-token';

/**
 * Get the stored authentication token
 * @returns {string|null} The auth token or null if not found
 */
export function getAuthToken() {
    return localStorage.getItem(TOKEN_KEY);
}

/**
 * Store the authentication token
 * @param {string} token - The token to store
 */
export function setAuthToken(token) {
    localStorage.setItem(TOKEN_KEY, token);
}

/**
 * Clear the authentication token
 */
export function clearAuthToken() {
    localStorage.removeItem(TOKEN_KEY);
}

/**
 * Get headers object with Authorization if token exists
 * @returns {Object} Headers object
 */
export function getAuthHeaders() {
    const headers = {};
    const token = getAuthToken();
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    return headers;
}

/**
 * Check if user is authenticated and load user data
 * @returns {Promise<boolean>} True if authenticated
 */
export async function checkAuth() {
    const token = getAuthToken();
    if (!token) {
        window.location.href = '/auth';
        return false;
    }

    try {
        const res = await fetch(`${API_BASE}/auth/me`, {
            headers: getAuthHeaders()
        });
        if (res.ok) {
            const data = await res.json();
            setCurrentUser(data.user);
            return true;
        } else {
            clearAuthToken();
            window.location.href = '/auth';
            return false;
        }
    } catch (err) {
        window.location.href = '/auth';
        return false;
    }
}

/**
 * Log out the current user
 * @param {Function} onLogout - Callback to run after logout
 */
export function logout(onLogout) {
    clearAuthToken();
    setCurrentUser(null);
    if (onLogout) {
        onLogout();
    }
}

/**
 * Render the user section in the header
 * @param {Object|null} user - User object or null
 */
export function renderUserSection(user) {
    const section = document.getElementById('userSection');
    if (!section) return;

    if (user) {
        section.innerHTML = DOMPurify.sanitize(`
            <span class="user-info">Signed in as <strong>${user.username}</strong></span>
            <button class="auth-btn" id="logoutBtn">Sign Out</button>
        `);
        // Add logout handler
        const logoutBtn = document.getElementById('logoutBtn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', () => logout());
        }
    } else {
        section.innerHTML = DOMPurify.sanitize(`
            <a href="/auth" class="auth-btn primary">Sign In</a>
        `);
    }
}
