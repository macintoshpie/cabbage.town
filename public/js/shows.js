// ============================================
// SHOWS & NOW PLAYING MODULE
// Handles show listings and live radio updates
// ============================================

import { setCurrentNowPlaying, setNowPlayingInterval, getRadioState } from './state.js';
import { playRecording, updateLiveFooterMetadata } from './player.js';

// Streamer mapping
// https://radio.cabbage.town/station/1/streamers
const STREAMER_MAP = {
    "DJ Ted": {
        "name": "mulch channel",
        "dj": "dj ted",
    },
    "DJ Chicago Style": {
        "name": "IS WiLD hour",
        "dj": "DJ CHICAGO STYLE",
    },
    "Reginajingles": {
        "name": "The reginajingles show",
        "dj": "reginajingles",
    },
    "Nights Like These": {
        "name": "Late Nights Like These",
        "dj": "Nights Like These",
    },
    "the conductor": {
        "name": "tracks from terminus",
        "dj": "the conductor",
    }
};

// ============================================
// NOW PLAYING FUNCTIONS
// ============================================

function updateLiveShowBanner(streamerDetails) {
    const liveShowBanner = document.getElementById('live-show-banner');
    if (!liveShowBanner) {
        return;
    }

    if (!streamerDetails) {
        liveShowBanner.style.display = 'none';
        return;
    }

    liveShowBanner.innerHTML = `(LIVE) ${streamerDetails.name} w/ ${streamerDetails.dj}`;
    liveShowBanner.style.display = 'inline-block';
}

function updateNowPlaying(nowPlaying) {
    if (!nowPlaying.streamerDetails) {
        const liveStreamerName = document.getElementById('liveStreamerName');
        if (liveStreamerName) {
            liveStreamerName.style.display = 'none';
        }
    } else {
        const liveStreamerName = document.getElementById('liveStreamerName');
        if (liveStreamerName) {
            liveStreamerName.innerHTML = `(LIVE) ${nowPlaying.streamerDetails.name} w/ ${nowPlaying.streamerDetails.dj}`;
            liveStreamerName.style.display = 'block';
        }
    }

    const nowPlayingArtwork = document.getElementById('nowPlayingArtwork');
    if (nowPlayingArtwork) {
        nowPlayingArtwork.src = nowPlaying.art;
        nowPlayingArtwork.alt = `${nowPlaying.artist} - ${nowPlaying.title}`;
    }
    
    const nowPlayingArtist = document.getElementById('nowPlayingArtist');
    if (nowPlayingArtist) {
        nowPlayingArtist.innerHTML = `<strong>${nowPlaying.artist}</strong>`;
    }
    
    const nowPlayingTitle = document.getElementById('nowPlayingTitle');
    if (nowPlayingTitle) {
        nowPlayingTitle.innerHTML = `<em>${nowPlaying.title}</em>`;
    }
}

function updateMediaSession(nowPlaying) {
    if (!('mediaSession' in navigator)) return;
    
    const mediaData = {
        title: nowPlaying.title,
        artist: nowPlaying.artist,
        artwork: [
            { src: nowPlaying.art, sizes: '512x512', type: 'image/png' }
        ]
    };
    
    if (nowPlaying.streamerDetails) {
        mediaData.title = `LIVE: ${nowPlaying.streamerDetails.name} w/ ${nowPlaying.streamerDetails.dj}`;
        mediaData.artist = nowPlaying.streamerDetails.dj;
        mediaData.artwork = [
            { src: '/the-cabbage.png', sizes: '512x512', type: 'image/png' }
        ];
    }
    
    // Only update if media session exists (playback state is managed separately)
    if (navigator.mediaSession.metadata) {
        navigator.mediaSession.metadata = new MediaMetadata({
            title: mediaData.title,
            artist: mediaData.artist,
            artwork: mediaData.artwork
        });
    }
}

async function fetchNowPlaying() {
    const res = await fetch('https://radio.cabbage.town/api/nowplaying/cabbage.town', {
        method: 'GET'
    });

    if (!res.ok) {
        console.error('Error fetching now playing data:', res.statusText);
        return;
    }

    const data = await res.json();
    console.log('Now playing data:', data);
    const live = data.live;
    let streamerDetails = undefined;
    if (live.is_live) {
        streamerDetails = STREAMER_MAP[live.streamer_name];
        if (!streamerDetails) {
            console.warn('Unknown streamer:', live.streamer_name);
            streamerDetails = {
                name: live.streamer_name,
                dj: live.streamer_name,
            };
        }
    }

    const song = data.now_playing.song;
    const artist = song.artist;
    const title = song.title;
    const art = song.art;

    return {
        artist: artist,
        title: title,
        art: art,
        streamerDetails: streamerDetails,
    };
}

export async function refreshNowPlaying() {
    const nowPlaying = await fetchNowPlaying();
    
    // Store for use by player
    setCurrentNowPlaying(nowPlaying);
    
    updateLiveShowBanner(nowPlaying.streamerDetails);
    updateNowPlaying(nowPlaying);
    updateMediaSession(nowPlaying);
    
    // Update footer if live radio is playing
    updateLiveFooterMetadata(nowPlaying);
}

// ============================================
// SHOWS LIST FUNCTIONS
// ============================================

export async function fetchAndDisplayShows() {
    const showsList = document.getElementById('shows-list');
    if (!showsList) {
        console.log('[Shows] No shows list found, skipping');
        return;
    }
    
    try {
        const response = await fetch('/shows.json?_=' + Date.now());
        if (!response.ok) {
            throw new Error('Failed to load shows');
        }
        
        const shows = await response.json();
        
        if (shows.length === 0) {
            showsList.innerHTML = '<p style="color: #666; font-style: italic;">No shows yet. Check back soon!</p>';
            return;
        }
        
        showsList.innerHTML = '';

        shows.forEach(show => {
            const showElement = document.createElement('div');
            showElement.style.cssText = `
                display: flex;
                justify-content: space-between;
                align-items: flex-start;
                gap: 12px;
                padding: 8px;
                border-radius: 8px;
                transition: background-color 0.2s;
            `;

            // Create info section (left side)
            const infoSection = document.createElement('div');
            infoSection.style.cssText = `
                display: flex;
                flex-direction: column;
                gap: 6px;
                flex-grow: 1;
            `;

            let infoHTML = `
                <div style="font-family: 'Courier New', monospace; font-weight: bold; font-size: 1.05em;">${show.title}</div>
                <div style="color: #666; font-family: 'Courier New', monospace; font-size: 0.9em;">
                    ${show.author ? `<span style="color: #444;">${show.author}</span> - ` : ''}${show.date}
                </div>
            `;

            // Add post excerpt and link if available
            if (show.post) {
                const linkText = show.recording ? 'Show Notes' : 'Read More';
                infoHTML += `
                    <div style="color: #555; font-family: 'Courier New', monospace; font-size: 0.85em; margin-top: 4px;">
                        ${show.post.excerpt}
                    </div>
                    <div style="margin-top: 4px;">
                        <a href="/patch/${show.post.slug}.html" style="color: var(--daorange); text-decoration: none; font-size: 0.9em; font-weight: bold;">
                            ${linkText}
                        </a>
                    </div>
                `;
            }

            infoSection.innerHTML = infoHTML;

            // Create play button (right side) if recording exists
            if (show.recording) {
                const playButton = document.createElement('button');
                playButton.style.cssText = `
                    background: black;
                    border: none;
                    border-radius: 50%;
                    width: 24px;
                    height: 24px;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    cursor: pointer;
                    flex-shrink: 0;
                    padding: 0;
                    margin-top: 4px;
                `;

                playButton.innerHTML = `
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <path d="M8 5v14l11-7z" fill="white"/>
                    </svg>
                `;

                playButton.onclick = () => playRecording(show, playButton);

                showElement.appendChild(infoSection);
                showElement.appendChild(playButton);
            } else {
                // No recording, just show the info
                showElement.appendChild(infoSection);
            }

            showsList.appendChild(showElement);
        });

    } catch (error) {
        console.error('Error fetching shows:', error);
        showsList.innerHTML = '<p style="color: red;">Error loading shows. Please try again later.</p>';
    }
}

// ============================================
// INITIALIZATION
// ============================================

export function initShows() {
    console.log('[Shows] Initializing shows module');
    
    // Check if this page has now playing elements (home page)
    const nowPlayingSection = document.getElementById('nowPlayingArtist');
    if (nowPlayingSection) {
        console.log('[Shows] Starting now playing updates');
        
        // Start now playing updates
        (async () => {
            await refreshNowPlaying();
            const interval = setInterval(refreshNowPlaying, 1000 * 15);
            setNowPlayingInterval(interval);
        })();
    }
    
    // Check if shows list exists and initialize if needed
    const showsList = document.getElementById('shows-list');
    if (showsList) {
        console.log('[Shows] Initializing shows list');
        fetchAndDisplayShows();
    }
    
    console.log('[Shows] Shows module initialized');
}

// Initialize patch page recording player buttons
// This should be called when navigating to a patch page
export function initPatchPagePlayer() {
    const playButtons = document.querySelectorAll('.recording-play-button');
    
    playButtons.forEach(button => {
        if (button.onclick) return; // Already initialized
        
        const recordingUrl = button.getAttribute('data-recording-url');
        const showTitle = button.getAttribute('data-show-title');
        const showAuthor = button.getAttribute('data-show-author');
        const showDate = button.getAttribute('data-show-date');
        
        if (!recordingUrl) {
            console.warn('[Shows] Play button missing data-recording-url');
            return;
        }
        
        // Create show object for player
        const show = {
            title: showTitle || 'Unknown Show',
            author: showAuthor || '',
            date: showDate || '',
            recording: {
                url: recordingUrl
            }
        };
        
        console.log('[Shows] Initializing patch page player button:', show.title);
        button.onclick = () => playRecording(show, button);
    });
}

