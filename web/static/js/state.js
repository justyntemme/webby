/**
 * Webby Library - State Module
 * Centralized application state management
 */

import { VIEW_MODES } from './config.js';

// Application state
const state = {
    books: [],
    viewMode: VIEW_MODES.GRID,
    groupBy: null,
    currentUser: null,
    groupSeries: false,
    expandedSeries: null,

    // Mobile action menu state
    mobileActionBookId: null,
    mobileActionBookData: null,
    longPressTimer: null,

    // Current book context
    currentBookId: null,
    currentBook: null,

    // Tags
    userTags: [],
    bookTags: [],

    // Search state
    searchResults: [],
    selectedResultIndex: -1,

    // Upload state
    uploadQueue: [],
    uploadProgress: {},

    // Collections
    collections: [],

    // Reading lists
    readingLists: {
        'want-to-read': [],
        'currently-reading': [],
        'finished': []
    }
};

// State getters
export function getState(key) {
    return state[key];
}

// State setters
export function setState(key, value) {
    state[key] = value;
}

// Batch state update
export function updateState(updates) {
    Object.assign(state, updates);
}

// Specific getters for common state
export function getBooks() {
    return state.books;
}

export function setBooks(books) {
    state.books = books;
}

export function getCurrentUser() {
    return state.currentUser;
}

export function setCurrentUser(user) {
    state.currentUser = user;
}

export function getViewMode() {
    return state.viewMode;
}

export function setViewMode(mode) {
    state.viewMode = mode;
}

export function getCurrentBook() {
    return state.currentBook;
}

export function setCurrentBook(book) {
    state.currentBook = book;
    state.currentBookId = book ? book.id : null;
}

export function getCurrentBookId() {
    return state.currentBookId;
}

export function getUserTags() {
    return state.userTags;
}

export function setUserTags(tags) {
    state.userTags = tags;
}

export function getBookTags() {
    return state.bookTags;
}

export function setBookTags(tags) {
    state.bookTags = tags;
}

// Export default state for debugging
export default state;
