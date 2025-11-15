// ============================================
// MAIN APP COORDINATOR
// Initializes and coordinates all modules
// ============================================

import { initState } from './state.js';
import { initPlayer } from './player.js';
import { initShows, initPatchPagePlayer } from './shows.js';

// ============================================
// PAGE-SPECIFIC INITIALIZATION
// ============================================

export function reinitPage() {
    console.log('[App] Reinitializing page-specific features');
    
    // Re-initialize player module (re-attaches radio control event listeners)
    initPlayer();
    
    // Re-initialize shows module (handles both shows list and now playing)
    initShows();
    
    // Initialize patch page recording player buttons if present
    initPatchPagePlayer();
    
    console.log('[App] Page reinitialization complete');
}

// ============================================
// MAIN INITIALIZATION
// ============================================

export function init() {
    console.log('[App] Initializing application');
    
    // Initialize modules in order
    initState();
    initPlayer();
    initShows();
    
    // Initialize patch page recording player buttons if present on initial load
    initPatchPagePlayer();
    
    // Router is initialized separately via router.js (it auto-inits)
    // Make reinitPage globally accessible for router
    window.reinitPage = reinitPage;
    
    console.log('[App] Application initialized successfully');
}

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}

