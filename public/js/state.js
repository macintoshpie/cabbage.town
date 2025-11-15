// ============================================
// CENTRALIZED STATE MANAGEMENT
// Single source of truth for app state
// ============================================

const state = {
    // Player state
    player: {
        type: null, // 'live' or 'recording'
        audioElement: null,
        isPlaying: false,
        metadata: {
            title: '',
            artist: '',
            date: '',
            artwork: ''
        }
    },
    
    // Page state
    page: {
        nowPlayingInterval: null,
        currentPath: window.location.pathname
    },
    
    // Radio state (now playing data from API)
    radio: {
        currentNowPlaying: null
    }
};

// Simple event emitter for state changes
const listeners = {};

function emit(event, data) {
    if (listeners[event]) {
        listeners[event].forEach(callback => callback(data));
    }
}

function on(event, callback) {
    if (!listeners[event]) {
        listeners[event] = [];
    }
    listeners[event].push(callback);
}

// State getters
export function getPlayerState() {
    return state.player;
}

export function getPageState() {
    return state.page;
}

export function getRadioState() {
    return state.radio;
}

// Player state setters
export function setPlayerType(type) {
    state.player.type = type;
    emit('player:typeChanged', type);
}

export function setAudioElement(element) {
    state.player.audioElement = element;
    emit('player:audioElementChanged', element);
}

export function setPlaying(isPlaying) {
    state.player.isPlaying = isPlaying;
    emit('player:playingChanged', isPlaying);
}

export function setPlayerMetadata(metadata) {
    state.player.metadata = { ...state.player.metadata, ...metadata };
    emit('player:metadataChanged', state.player.metadata);
}

export function resetPlayer() {
    state.player.type = null;
    state.player.audioElement = null;
    state.player.isPlaying = false;
    emit('player:reset');
}

// Page state setters
export function setNowPlayingInterval(interval) {
    if (state.page.nowPlayingInterval) {
        clearInterval(state.page.nowPlayingInterval);
    }
    state.page.nowPlayingInterval = interval;
}

export function setCurrentPath(path) {
    state.page.currentPath = path;
    emit('page:pathChanged', path);
}

// Radio state setters
export function setCurrentNowPlaying(data) {
    state.radio.currentNowPlaying = data;
    emit('radio:nowPlayingChanged', data);
}

// Event listener management
export function addEventListener(event, callback) {
    on(event, callback);
}

export function removeEventListener(event, callback) {
    if (listeners[event]) {
        listeners[event] = listeners[event].filter(cb => cb !== callback);
    }
}

// Initialize state
export function initState() {
    console.log('[State] State management initialized');
    return state;
}

// Export state for debugging (read-only access)
export function getState() {
    return JSON.parse(JSON.stringify(state)); // Deep clone for safety
}

