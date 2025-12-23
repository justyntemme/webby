/**
 * Webby Library - Utility Functions
 * Common helper functions used across the application
 */

/**
 * Format bytes to human-readable size
 * @param {number} bytes - Size in bytes
 * @returns {string} Formatted size string
 */
export function formatFileSize(bytes) {
    if (!bytes) return 'Unknown';
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return Math.round(bytes / Math.pow(1024, i) * 100) / 100 + ' ' + sizes[i];
}

/**
 * Validate ISBN format
 * @param {string} isbn - ISBN string to validate
 * @returns {boolean} True if valid
 */
export function validateISBN(isbn) {
    if (!isbn) return true; // Empty is ok
    const cleanISBN = isbn.replace(/[-\s]/g, '');
    if (cleanISBN.length === 10) {
        return /^[0-9]{9}[0-9X]$/.test(cleanISBN);
    }
    if (cleanISBN.length === 13) {
        return /^[0-9]{13}$/.test(cleanISBN);
    }
    return false;
}

/**
 * Render star rating HTML
 * @param {number} rating - Rating value (0-5)
 * @returns {string} HTML string for stars
 */
export function renderStarsHtml(rating) {
    let html = '';
    for (let i = 1; i <= 5; i++) {
        if (i <= rating) {
            html += '<span class="star filled">&#9733;</span>';
        } else {
            html += '<span class="star empty">&#9734;</span>';
        }
    }
    return html;
}

/**
 * Debounce a function
 * @param {Function} func - Function to debounce
 * @param {number} wait - Wait time in ms
 * @returns {Function} Debounced function
 */
export function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

/**
 * Throttle a function
 * @param {Function} func - Function to throttle
 * @param {number} limit - Time limit in ms
 * @returns {Function} Throttled function
 */
export function throttle(func, limit) {
    let inThrottle;
    return function(...args) {
        if (!inThrottle) {
            func.apply(this, args);
            inThrottle = true;
            setTimeout(() => inThrottle = false, limit);
        }
    };
}

/**
 * Safely set innerHTML with DOMPurify
 * @param {HTMLElement} element - Target element
 * @param {string} html - HTML content
 */
export function setInnerHTML(element, html) {
    if (element && typeof DOMPurify !== 'undefined') {
        element.innerHTML = DOMPurify.sanitize(html);
    } else if (element) {
        element.innerHTML = html;
    }
}

/**
 * Get element by ID with null check
 * @param {string} id - Element ID
 * @returns {HTMLElement|null}
 */
export function $(id) {
    return document.getElementById(id);
}

/**
 * Query selector shorthand
 * @param {string} selector - CSS selector
 * @param {HTMLElement} parent - Parent element (default: document)
 * @returns {HTMLElement|null}
 */
export function $$(selector, parent = document) {
    return parent.querySelector(selector);
}

/**
 * Query selector all shorthand
 * @param {string} selector - CSS selector
 * @param {HTMLElement} parent - Parent element (default: document)
 * @returns {NodeList}
 */
export function $$all(selector, parent = document) {
    return parent.querySelectorAll(selector);
}

/**
 * Add event listener with optional delegation
 * @param {HTMLElement} element - Target element
 * @param {string} event - Event type
 * @param {string|Function} selectorOrHandler - Selector for delegation or handler
 * @param {Function} handler - Handler function (if using delegation)
 */
export function on(element, event, selectorOrHandler, handler) {
    if (typeof selectorOrHandler === 'function') {
        element.addEventListener(event, selectorOrHandler);
    } else {
        element.addEventListener(event, (e) => {
            const target = e.target.closest(selectorOrHandler);
            if (target) {
                handler.call(target, e);
            }
        });
    }
}
