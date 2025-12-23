/**
 * Webby Library - Main Entry Point
 * Application initialization and event binding
 */

import { API_BASE, LONG_PRESS_DURATION, VIEW_MODES } from './config.js';
import {
    getState, setState, setBooks, getBooks,
    getCurrentUser, setCurrentUser, setCurrentBook,
    getViewMode, setViewMode, getUserTags, setUserTags,
    getBookTags, setBookTags
} from './state.js';
import {
    getAuthToken, getAuthHeaders, checkAuth,
    logout, renderUserSection
} from './auth.js';
import { apiGet, apiPost, apiPut, apiDelete, fetchJson, uploadFile } from './api.js';
import {
    formatFileSize, validateISBN, renderStarsHtml,
    debounce, throttle, setInnerHTML, $, $$, $$all, on
} from './utils.js';

// Re-export for global access during migration
window.webby = {
    // Config
    API_BASE,
    LONG_PRESS_DURATION,
    VIEW_MODES,

    // State
    getState,
    setState,
    setBooks,
    getBooks,
    getCurrentUser,
    setCurrentUser,
    setCurrentBook,
    getViewMode,
    setViewMode,
    getUserTags,
    setUserTags,
    getBookTags,
    setBookTags,

    // Auth
    getAuthToken,
    getAuthHeaders,
    checkAuth,
    logout,
    renderUserSection,

    // API
    apiGet,
    apiPost,
    apiPut,
    apiDelete,
    fetchJson,
    uploadFile,

    // Utils
    formatFileSize,
    validateISBN,
    renderStarsHtml,
    debounce,
    throttle,
    setInnerHTML,
    $,
    $$,
    $$all,
    on
};

/**
 * Initialize the application
 */
export async function init() {
    console.log('Webby Library initializing...');

    // Check authentication
    const isAuth = await checkAuth();
    if (!isAuth) return;

    // Render user section
    renderUserSection(getCurrentUser());

    console.log('Webby Library initialized');
}

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}

export default { init };
